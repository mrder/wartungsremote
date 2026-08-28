// Package support persists the dedicated local OS "remote-support"
// account credential the agent provisions on each device (see
// docs/AGENT.md "Remote-support account"). The SSH/RDP tunnel only
// forwards raw network traffic to the device's own existing SSH/RDP
// service — a separate login from our Ed25519 device identity — so
// without a known account, remote support would need the customer's own
// credentials, defeating the point. The password is encrypted at rest
// with AES-256-GCM using the same key as TOTP secrets
// (WR_TOTP_ENCRYPTION_KEY_FILE) and is only ever decrypted on an
// explicit, audited dashboard reveal.
package support

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("support: not found")

type Repo struct {
	pool *pgxpool.Pool
	key  []byte // 32 bytes, shared with auth.MFAStore
}

func NewRepo(pool *pgxpool.Pool, key []byte) *Repo {
	return &Repo{pool: pool, key: key}
}

func (r *Repo) encrypt(plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(r.key)
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

func (r *Repo) decrypt(ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(r.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// Upsert stores (or replaces, on rotation) the credential the agent just
// reported.
func (r *Repo) Upsert(ctx context.Context, deviceID uuid.UUID, username, password string) error {
	ciphertext, nonce, err := r.encrypt([]byte(password))
	if err != nil {
		return fmt.Errorf("support: encrypt: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO device_support_credentials (device_id, username, password_ciphertext, password_nonce, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (device_id) DO UPDATE SET
			username = EXCLUDED.username,
			password_ciphertext = EXCLUDED.password_ciphertext,
			password_nonce = EXCLUDED.password_nonce,
			updated_at = now()
	`, deviceID, username, ciphertext, nonce)
	if err != nil {
		return fmt.Errorf("support: upsert: %w", err)
	}
	return nil
}

// Credential is the decrypted, in-memory-only representation — never
// logged, never cached, fetched fresh on every reveal.
type Credential struct {
	Username  string
	Password  string
	UpdatedAt string
}

// Status is the lightweight, non-decrypting existence check used to show
// "is a remote-support login even usable for this device" in the
// dashboard overview, without paying the cost (or audit noise) of a full
// reveal just to answer that.
type Status struct {
	Available bool
	UpdatedAt string
}

func (r *Repo) GetStatus(ctx context.Context, deviceID uuid.UUID) (Status, error) {
	var updatedAt string
	err := r.pool.QueryRow(ctx, `
		SELECT updated_at::text FROM device_support_credentials WHERE device_id = $1
	`, deviceID).Scan(&updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{Available: false}, nil
	}
	if err != nil {
		return Status{}, fmt.Errorf("support: get status: %w", err)
	}
	return Status{Available: true, UpdatedAt: updatedAt}, nil
}

// ListDueForRotation returns device IDs whose credential was last set
// before the cutoff — used by RunRotationSweeper (rotation.go). A device
// with no reported credential yet is not "due" (it's simply not in this
// table at all); this only ever rotates something that already exists.
func (r *Repo) ListDueForRotation(ctx context.Context, olderThan time.Duration) ([]uuid.UUID, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	rows, err := r.pool.Query(ctx, `SELECT device_id FROM device_support_credentials WHERE updated_at < $1`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("support: list due for rotation: %w", err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("support: scan due-for-rotation row: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, deviceID uuid.UUID) (Credential, error) {
	var username string
	var ciphertext, nonce []byte
	var updatedAt string
	err := r.pool.QueryRow(ctx, `
		SELECT username, password_ciphertext, password_nonce, updated_at::text
		FROM device_support_credentials WHERE device_id = $1
	`, deviceID).Scan(&username, &ciphertext, &nonce, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, ErrNotFound
	}
	if err != nil {
		return Credential{}, fmt.Errorf("support: get: %w", err)
	}
	plaintext, err := r.decrypt(ciphertext, nonce)
	if err != nil {
		return Credential{}, fmt.Errorf("support: decrypt: %w", err)
	}
	return Credential{Username: username, Password: string(plaintext), UpdatedAt: updatedAt}, nil
}
