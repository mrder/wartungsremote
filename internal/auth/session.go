package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSessionInvalid = errors.New("auth: session invalid or expired")

type Session struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	CreatedAt     time.Time
	LastSeenAt    time.Time
	ExpiresAt     time.Time
	IdleExpiresAt time.Time
}

type SessionStore struct {
	pool       *pgxpool.Pool
	cookieName string
	absoluteTTL time.Duration
	idleTTL     time.Duration
}

func NewSessionStore(pool *pgxpool.Pool, cookieName string, absoluteTTL, idleTTL time.Duration) *SessionStore {
	return &SessionStore{pool: pool, cookieName: cookieName, absoluteTTL: absoluteTTL, idleTTL: idleTTL}
}

// newOpaqueToken returns a >=128 bit CSPRNG token and its SHA-256 hash for
// storage, per docs/SECURITY.md §9 (only the hash is ever persisted).
func newOpaqueToken() (token string, hash []byte, err error) {
	raw := make([]byte, 32) // 256 bit
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("auth: generate session token: %w", err)
	}
	token = encodeToken(raw)
	// Hash the *encoded* token (not raw) so this matches hashToken() used by
	// Validate() — encodeToken is a lossy mod-alphabet mapping, so hashing
	// raw and hashing the encoded string are NOT interchangeable.
	return token, hashToken(token), nil
}

func encodeToken(raw []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	out := make([]byte, 0, len(raw)*2)
	for _, b := range raw {
		out = append(out, alphabet[int(b)%len(alphabet)])
	}
	return string(out)
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Create starts a new session for userID, invalidating no prior sessions
// (multiple concurrent sessions are permitted; use RevokeAllForUser to force
// logout everywhere, e.g. on incident response).
func (s *SessionStore) Create(ctx context.Context, userID uuid.UUID, ip, userAgent string) (token string, sess Session, err error) {
	token, hash, err := newOpaqueToken()
	if err != nil {
		return "", Session{}, err
	}
	now := time.Now().UTC()
	sess = Session{
		UserID:        userID,
		CreatedAt:     now,
		LastSeenAt:    now,
		ExpiresAt:     now.Add(s.absoluteTTL),
		IdleExpiresAt: now.Add(s.idleTTL),
	}
	var ipVal any
	if ip != "" {
		ipVal = ip
	}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO user_sessions (user_id, session_token_hash, created_at, last_seen_at, expires_at, idle_expires_at, ip, user_agent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id
	`, userID, hash, sess.CreatedAt, sess.LastSeenAt, sess.ExpiresAt, sess.IdleExpiresAt, ipVal, userAgent).Scan(&sess.ID)
	if err != nil {
		return "", Session{}, fmt.Errorf("auth: create session: %w", err)
	}
	return token, sess, nil
}

// Validate looks up a session by its opaque token, verifies expiry, and
// slides the idle timeout forward.
func (s *SessionStore) Validate(ctx context.Context, token string) (Session, error) {
	hash := hashToken(token)
	var sess Session
	var revokedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, created_at, last_seen_at, expires_at, idle_expires_at, revoked_at
		FROM user_sessions WHERE session_token_hash = $1
	`, hash).Scan(&sess.ID, &sess.UserID, &sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt, &sess.IdleExpiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionInvalid
	}
	if err != nil {
		return Session{}, fmt.Errorf("auth: validate session: %w", err)
	}
	now := time.Now().UTC()
	if revokedAt != nil || now.After(sess.ExpiresAt) || now.After(sess.IdleExpiresAt) {
		return Session{}, ErrSessionInvalid
	}

	newIdle := now.Add(s.idleTTL)
	if newIdle.After(sess.ExpiresAt) {
		newIdle = sess.ExpiresAt
	}
	_, err = s.pool.Exec(ctx, `UPDATE user_sessions SET last_seen_at = $2, idle_expires_at = $3 WHERE id = $1`, sess.ID, now, newIdle)
	if err != nil {
		return Session{}, fmt.Errorf("auth: slide session: %w", err)
	}
	sess.LastSeenAt = now
	sess.IdleExpiresAt = newIdle
	return sess, nil
}

func (s *SessionStore) Revoke(ctx context.Context, sessionID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE user_sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, sessionID)
	if err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	return nil
}

// RevokeAllForUser invalidates every active session for a user; used for
// incident response and on credential-relevant changes.
func (s *SessionStore) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE user_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("auth: revoke all sessions: %w", err)
	}
	return nil
}

// SetCookie writes the session cookie following docs/SECURITY.md §9:
// __Host- prefix, Secure, HttpOnly, SameSite=Strict, Path=/.
func (s *SessionStore) SetCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *SessionStore) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *SessionStore) ReadCookie(r *http.Request) (string, error) {
	c, err := r.Cookie(s.cookieName)
	if err != nil {
		return "", ErrSessionInvalid
	}
	return c.Value, nil
}
