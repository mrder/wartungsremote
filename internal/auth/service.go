package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service bundles the pieces needed by the HTTP layer to implement
// docs/API.md §2-3 (auth) end to end.
type Service struct {
	pool           *pgxpool.Pool
	Repo           *Repo
	Sessions       *SessionStore
	MFA            *MFAStore
	Argon2         Argon2Params
	LoginLimiter   *RateLimiter
	MFALimiter     *RateLimiter
	PrivilegeTTL   time.Duration
	Issuer         string
}

func NewService(pool *pgxpool.Pool, sessions *SessionStore, mfa *MFAStore, argon2 Argon2Params, privilegeTTL time.Duration, issuer string) *Service {
	return &Service{
		pool:         pool,
		Repo:         NewRepo(pool),
		Sessions:     sessions,
		MFA:          mfa,
		Argon2:       argon2,
		LoginLimiter: NewRateLimiter(10, time.Minute),
		MFALimiter:   NewRateLimiter(5, time.Minute),
		PrivilegeTTL: privilegeTTL,
		Issuer:       issuer,
	}
}

var ErrInvalidCredentials = errors.New("auth: invalid credentials")
var ErrRateLimited = errors.New("auth: rate limited")
var ErrMFASetupRequired = errors.New("auth: mfa setup required")

// LoginResult communicates what the caller must do next.
type LoginResult struct {
	State       string // "mfa_required" | "mfa_setup_required" | "authenticated"
	ChallengeID string
	SetupURI    string
	Token       string
	Session     Session
}

// Login validates username+password. It never reveals whether the username
// or the password was wrong (uniform ErrInvalidCredentials), and always
// consults the rate limiter first.
func (s *Service) Login(ctx context.Context, username, password, ip, userAgent string, requireMFA bool) (LoginResult, error) {
	if !s.LoginLimiter.Allow("login:" + ip) {
		return LoginResult{}, ErrRateLimited
	}

	user, err := s.Repo.GetByUsername(ctx, username)
	if err != nil {
		// Constant-effort path: still perform a hash comparison against a
		// fixed dummy hash so timing does not reveal username existence.
		_, _ = VerifyPassword(password, dummyHash)
		return LoginResult{}, ErrInvalidCredentials
	}

	if user.Status != UserActive {
		return LoginResult{}, ErrInvalidCredentials
	}
	if user.LockedUntil != nil && time.Now().UTC().Before(*user.LockedUntil) {
		return LoginResult{}, ErrLocked
	}

	ok, err := VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: verify password: %w", err)
	}
	if !ok {
		_ = s.Repo.RegisterFailedLogin(ctx, user.ID)
		return LoginResult{}, ErrInvalidCredentials
	}

	confirmed, err := s.MFA.IsConfirmed(ctx, user.ID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: check mfa: %w", err)
	}

	if !confirmed {
		if requireMFA || user.MFARequired {
			uri, err := s.MFA.BeginSetup(ctx, user.ID, user.Username, s.Issuer)
			if err != nil {
				return LoginResult{}, fmt.Errorf("auth: begin mfa setup: %w", err)
			}
			return LoginResult{State: "mfa_setup_required", SetupURI: uri}, nil
		}
		_ = s.Repo.ResetFailedLogins(ctx, user.ID)
		token, sess, err := s.Sessions.Create(ctx, user.ID, ip, userAgent)
		if err != nil {
			return LoginResult{}, err
		}
		return LoginResult{State: "authenticated", Token: token, Session: sess}, nil
	}

	challengeID, err := s.createMFAChallenge(ctx, user.ID, ip)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{State: "mfa_required", ChallengeID: challengeID}, nil
}

// dummyHash is a valid-format Argon2id hash used only for constant-effort
// verification when a username does not exist, so login timing does not
// leak account existence.
const dummyHash = "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func (s *Service) createMFAChallenge(ctx context.Context, userID uuid.UUID, ip string) (string, error) {
	var id uuid.UUID
	var ipVal any
	if ip != "" {
		ipVal = ip
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO mfa_challenges (user_id, expires_at, ip)
		VALUES ($1, now() + interval '5 minutes', $2)
		RETURNING id
	`, userID, ipVal).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("auth: create mfa challenge: %w", err)
	}
	return id.String(), nil
}

// CompleteMFA validates a TOTP code against a pending login challenge and,
// on success, creates the session.
func (s *Service) CompleteMFA(ctx context.Context, challengeID, code, ip, userAgent string) (LoginResult, error) {
	if !s.MFALimiter.Allow("mfa:" + challengeID) {
		return LoginResult{}, ErrRateLimited
	}

	cid, err := uuid.Parse(challengeID)
	if err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	var userID uuid.UUID
	var expiresAt time.Time
	var consumedAt *time.Time
	var attempts int
	err = s.pool.QueryRow(ctx, `
		SELECT user_id, expires_at, consumed_at, attempts FROM mfa_challenges WHERE id = $1
	`, cid).Scan(&userID, &expiresAt, &consumedAt, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: load mfa challenge: %w", err)
	}
	if consumedAt != nil || time.Now().UTC().After(expiresAt) || attempts >= 5 {
		return LoginResult{}, ErrInvalidCredentials
	}

	valid, err := s.MFA.ValidateCode(ctx, userID, code)
	if err != nil && !errors.Is(err, ErrMFANotConfigured) {
		return LoginResult{}, fmt.Errorf("auth: validate mfa code: %w", err)
	}
	if !valid {
		_, _ = s.pool.Exec(ctx, `UPDATE mfa_challenges SET attempts = attempts + 1 WHERE id = $1`, cid)
		return LoginResult{}, ErrInvalidCredentials
	}

	_, err = s.pool.Exec(ctx, `UPDATE mfa_challenges SET consumed_at = now() WHERE id = $1`, cid)
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: consume mfa challenge: %w", err)
	}

	_ = s.Repo.ResetFailedLogins(ctx, userID)
	token, sess, err := s.Sessions.Create(ctx, userID, ip, userAgent)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{State: "authenticated", Token: token, Session: sess}, nil
}

// ConfirmMFASetup finalizes MFA enrollment for a user who is mid-login with
// state mfa_setup_required. It requires the username+password to have
// already been verified by the caller in this same request (see handlers.go)
// since no session exists yet at setup time.
func (s *Service) ConfirmMFASetup(ctx context.Context, userID uuid.UUID, code string) ([]string, error) {
	return s.MFA.ConfirmSetup(ctx, userID, code)
}

// Reauth re-verifies password+TOTP for an already-authenticated session and
// issues a short-lived reauth_id, per docs/API.md §2 POST /auth/reauth. It
// does not return or extend the main session token.
func (s *Service) Reauth(ctx context.Context, userID uuid.UUID, password, code, ip string) (string, error) {
	if !s.MFALimiter.Allow("reauth:" + userID.String()) {
		return "", ErrRateLimited
	}
	user, err := s.Repo.GetByID(ctx, userID)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	ok, err := VerifyPassword(password, user.PasswordHash)
	if err != nil || !ok {
		return "", ErrInvalidCredentials
	}
	valid, err := s.MFA.ValidateCode(ctx, userID, code)
	if err != nil || !valid {
		return "", ErrInvalidCredentials
	}

	var id uuid.UUID
	var ipVal any
	if ip != "" {
		ipVal = ip
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO reauth_tokens (user_id, expires_at, ip)
		VALUES ($1, now() + interval '5 minutes', $2)
		RETURNING id
	`, userID, ipVal).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("auth: create reauth token: %w", err)
	}
	return id.String(), nil
}

// ConsumeReauth validates and single-use-consumes a reauth_id for a given
// user, returning true if it was valid and freshly consumed.
func (s *Service) ConsumeReauth(ctx context.Context, userID uuid.UUID, reauthID string) (bool, error) {
	id, err := uuid.Parse(reauthID)
	if err != nil {
		return false, nil
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE reauth_tokens SET consumed_at = now()
		WHERE id = $1 AND user_id = $2 AND consumed_at IS NULL AND expires_at > now()
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("auth: consume reauth token: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ErrPasswordTooShort mirrors the minimum enforced at admin bootstrap
// (docs/SECURITY.md §7) so the rule is the same everywhere a password is
// ever set, not just at createadmin time.
var ErrPasswordTooShort = errors.New("auth: password must be at least 12 characters")

// ChangeOwnPassword sets a new password for an already-authenticated user.
// The caller (HTTP handler) is responsible for having already consumed a
// fresh ConsumeReauth (current password + MFA) — this method only
// enforces the password policy and writes the new hash; it does not
// re-verify identity itself, since reauth already did.
func (s *Service) ChangeOwnPassword(ctx context.Context, userID uuid.UUID, newPassword string) error {
	if len(newPassword) < 12 {
		return ErrPasswordTooShort
	}
	hash, err := HashPassword(newPassword, s.Argon2)
	if err != nil {
		return fmt.Errorf("auth: hash new password: %w", err)
	}
	return s.Repo.UpdatePasswordHash(ctx, userID, hash)
}

// HashRandomSecret is a small helper for generating opaque high-entropy
// identifiers elsewhere (e.g. enrollment tokens) with a consistent
// SHA-256-based storage hash.
func HashRandomSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// NewRandomSecret returns n random bytes suitable for tokens/tickets.
func NewRandomSecret(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("auth: random secret: %w", err)
	}
	return b, nil
}
