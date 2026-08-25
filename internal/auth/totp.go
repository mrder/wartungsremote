package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

var ErrMFANotConfigured = errors.New("auth: mfa not configured")
var ErrMFAAlreadyConfirmed = errors.New("auth: mfa already confirmed")

// MFAStore persists encrypted TOTP secrets and hashed recovery codes.
// TOTP secret is encrypted at rest with AES-256-GCM using the server's
// WR_TOTP_ENCRYPTION_KEY_FILE, per docs/SECURITY.md §8.
type MFAStore struct {
	pool *pgxpool.Pool
	key  []byte // 32 bytes
}

func NewMFAStore(pool *pgxpool.Pool, key []byte) *MFAStore {
	return &MFAStore{pool: pool, key: key}
}

func (s *MFAStore) encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func (s *MFAStore) decrypt(ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// BeginSetup generates a new TOTP secret, encrypts and stores it as
// unconfirmed, and returns the provisioning URI to render as a QR code.
// The QR/URI must only be shown once, during this setup flow.
func (s *MFAStore) BeginSetup(ctx context.Context, userID uuid.UUID, username, issuer string) (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: username,
	})
	if err != nil {
		return "", fmt.Errorf("auth: generate totp secret: %w", err)
	}

	ciphertext, nonce, err := s.encrypt([]byte(key.Secret()))
	if err != nil {
		return "", fmt.Errorf("auth: encrypt totp secret: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO user_mfa (user_id, secret_ciphertext, secret_nonce, secret_key_version, confirmed_at, recovery_code_hashes)
		VALUES ($1, $2, $3, 1, NULL, '[]'::jsonb)
		ON CONFLICT (user_id) DO UPDATE SET
			secret_ciphertext = EXCLUDED.secret_ciphertext,
			secret_nonce = EXCLUDED.secret_nonce,
			confirmed_at = NULL,
			recovery_code_hashes = '[]'::jsonb,
			updated_at = now()
		WHERE user_mfa.confirmed_at IS NULL
	`, userID, ciphertext, nonce)
	if err != nil {
		return "", fmt.Errorf("auth: store totp secret: %w", err)
	}
	return key.URL(), nil
}

func (s *MFAStore) loadSecret(ctx context.Context, userID uuid.UUID) (secret string, confirmed bool, err error) {
	var ciphertext, nonce []byte
	var confirmedAt *time.Time
	err = s.pool.QueryRow(ctx, `
		SELECT secret_ciphertext, secret_nonce, confirmed_at FROM user_mfa WHERE user_id = $1
	`, userID).Scan(&ciphertext, &nonce, &confirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, ErrMFANotConfigured
	}
	if err != nil {
		return "", false, fmt.Errorf("auth: load totp secret: %w", err)
	}
	plain, err := s.decrypt(ciphertext, nonce)
	if err != nil {
		return "", false, fmt.Errorf("auth: decrypt totp secret: %w", err)
	}
	return string(plain), confirmedAt != nil, nil
}

// ConfirmSetup validates the first code and marks MFA confirmed, then
// generates recovery codes (returned once, in plaintext, to be shown to the
// admin exactly once).
func (s *MFAStore) ConfirmSetup(ctx context.Context, userID uuid.UUID, code string) ([]string, error) {
	secret, confirmed, err := s.loadSecret(ctx, userID)
	if err != nil {
		return nil, err
	}
	if confirmed {
		return nil, ErrMFAAlreadyConfirmed
	}
	if !totp.Validate(code, secret) {
		return nil, ErrInvalidCode
	}

	codes, hashes, err := generateRecoveryCodes(10)
	if err != nil {
		return nil, err
	}
	hashesJSON, err := json.Marshal(hashes)
	if err != nil {
		return nil, err
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE user_mfa SET confirmed_at = now(), recovery_code_hashes = $2, updated_at = now()
		WHERE user_id = $1
	`, userID, hashesJSON)
	if err != nil {
		return nil, fmt.Errorf("auth: confirm totp: %w", err)
	}
	return codes, nil
}

var ErrInvalidCode = errors.New("auth: invalid mfa code")

// ValidateCode checks a live TOTP code for an already-confirmed user.
func (s *MFAStore) ValidateCode(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	secret, confirmed, err := s.loadSecret(ctx, userID)
	if err != nil {
		return false, err
	}
	if !confirmed {
		return false, ErrMFANotConfigured
	}
	valid, err := totp.ValidateCustom(code, secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return false, nil
	}
	return valid, nil
}

// IsConfirmed reports whether a user has completed TOTP setup.
func (s *MFAStore) IsConfirmed(ctx context.Context, userID uuid.UUID) (bool, error) {
	_, confirmed, err := s.loadSecret(ctx, userID)
	if errors.Is(err, ErrMFANotConfigured) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return confirmed, nil
}

// ConsumeRecoveryCode checks the code against stored hashes; on match it is
// removed (single use) and true is returned.
func (s *MFAStore) ConsumeRecoveryCode(ctx context.Context, userID uuid.UUID, code string) (bool, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT recovery_code_hashes FROM user_mfa WHERE user_id = $1`, userID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrMFANotConfigured
	}
	if err != nil {
		return false, fmt.Errorf("auth: load recovery codes: %w", err)
	}
	var hashes []string
	if err := json.Unmarshal(raw, &hashes); err != nil {
		return false, fmt.Errorf("auth: decode recovery codes: %w", err)
	}

	codeHash := hashRecoveryCode(code)
	idx := -1
	for i, h := range hashes {
		if subtle.ConstantTimeCompare([]byte(h), []byte(codeHash)) == 1 {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false, nil
	}
	hashes = append(hashes[:idx], hashes[idx+1:]...)
	newRaw, err := json.Marshal(hashes)
	if err != nil {
		return false, err
	}
	_, err = s.pool.Exec(ctx, `UPDATE user_mfa SET recovery_code_hashes = $2, updated_at = now() WHERE user_id = $1`, userID, newRaw)
	if err != nil {
		return false, fmt.Errorf("auth: consume recovery code: %w", err)
	}
	return true, nil
}

func generateRecoveryCodes(n int) (plaintext []string, hashes []string, err error) {
	for i := 0; i < n; i++ {
		buf := make([]byte, 8)
		if _, err := rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("auth: generate recovery code: %w", err)
		}
		code := hex.EncodeToString(buf)
		plaintext = append(plaintext, code)
		hashes = append(hashes, hashRecoveryCode(code))
	}
	return plaintext, hashes, nil
}

func hashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
