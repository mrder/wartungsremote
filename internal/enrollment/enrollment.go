// Package enrollment implements one-time device enrollment tokens and
// atomic consumption into a registered device, per docs/SPECIFICATION.md §8
// and docs/DATABASE.md §4.
package enrollment

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTokenInvalid  = errors.New("enrollment: token invalid, expired, or already used")
	ErrInstallExists = errors.New("enrollment: install_id already enrolled")
)

// CreateParams are the admin-supplied parameters for POST /api/v1/enrollments.
type CreateParams struct {
	CustomerID    *uuid.UUID
	GroupID       *uuid.UUID
	DisplayName   string
	ExpiresIn     time.Duration
	Tags          []string
	CreatedBy     uuid.UUID
}

// Created is returned once; the plaintext token is never retrievable again.
type Created struct {
	ID        uuid.UUID
	Token     string
	ExpiresAt time.Time
}

type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// tokenEntropyBytes: 256 bit CSPRNG entropy per docs/SPECIFICATION.md §8.
const tokenEntropyBytes = 32

func generateToken() (plaintext string, hash []byte, err error) {
	raw := make([]byte, tokenEntropyBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("enrollment: generate token: %w", err)
	}
	plaintext = "wr_enroll_" + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	return plaintext, sum[:], nil
}

func hashToken(token string) ([]byte, error) {
	const prefix = "wr_enroll_"
	if len(token) <= len(prefix) || token[:len(prefix)] != prefix {
		return nil, ErrTokenInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(token[len(prefix):])
	if err != nil {
		return nil, ErrTokenInvalid
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}

func (s *Service) Create(ctx context.Context, p CreateParams) (Created, error) {
	if p.ExpiresIn <= 0 || p.ExpiresIn > 24*time.Hour {
		p.ExpiresIn = 30 * time.Minute
	}
	token, hash, err := generateToken()
	if err != nil {
		return Created{}, err
	}
	tagsJSON, err := json.Marshal(p.Tags)
	if err != nil {
		return Created{}, fmt.Errorf("enrollment: marshal tags: %w", err)
	}

	var id uuid.UUID
	expiresAt := time.Now().UTC().Add(p.ExpiresIn)
	err = s.pool.QueryRow(ctx, `
		INSERT INTO enrollment_tokens (token_hash, customer_id, group_id, display_name, tags, created_by, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id
	`, hash, p.CustomerID, p.GroupID, p.DisplayName, tagsJSON, p.CreatedBy, expiresAt).Scan(&id)
	if err != nil {
		return Created{}, fmt.Errorf("enrollment: create token: %w", err)
	}
	return Created{ID: id, Token: token, ExpiresAt: expiresAt}, nil
}

func (s *Service) Revoke(ctx context.Context, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE enrollment_tokens SET revoked_at = now()
		WHERE id = $1 AND consumed_at IS NULL AND revoked_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("enrollment: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTokenInvalid
	}
	return nil
}

// RevokeAllOutstanding revokes every not-yet-consumed, not-yet-revoked
// enrollment token, regardless of expiry. Incident response tool
// (docs/SECURITY.md §20 "Enrollment Tokens global widerrufen") for e.g. a
// suspected leak of a token distribution channel.
func (s *Service) RevokeAllOutstanding(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE enrollment_tokens SET revoked_at = now()
		WHERE consumed_at IS NULL AND revoked_at IS NULL
	`)
	if err != nil {
		return 0, fmt.Errorf("enrollment: revoke all outstanding: %w", err)
	}
	return tag.RowsAffected(), nil
}

// AgentEnrollRequest is what the agent submits to POST /api/v1/agent/enroll.
type AgentEnrollRequest struct {
	Token        string
	InstallID    uuid.UUID
	PublicKey    []byte // Ed25519 public key, 32 bytes
	AgentVersion string
	OS           string
	Arch         string
	Hostname     string
}

// Enrolled is the server's response: the newly assigned device identity.
type Enrolled struct {
	DeviceID uuid.UUID
}

// Consume atomically validates+consumes the token and creates the device +
// its initial credential, per docs/SPECIFICATION.md §8 steps 5-9. The whole
// operation runs in a single transaction so a token can never be consumed
// twice even under concurrent enroll attempts (SELECT ... FOR UPDATE
// serializes racing consumers on the same token row).
func (s *Service) Consume(ctx context.Context, req AgentEnrollRequest) (Enrolled, error) {
	if len(req.PublicKey) != 32 {
		return Enrolled{}, fmt.Errorf("enrollment: %w: public key must be 32 bytes (Ed25519)", ErrTokenInvalid)
	}
	hash, err := hashToken(req.Token)
	if err != nil {
		return Enrolled{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Enrolled{}, fmt.Errorf("enrollment: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var tokenID uuid.UUID
	var customerID, groupID *uuid.UUID
	var displayName *string
	var tagsRaw []byte
	var expiresAt time.Time
	var consumedAt, revokedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT id, customer_id, group_id, display_name, tags, expires_at, consumed_at, revoked_at
		FROM enrollment_tokens WHERE token_hash = $1 FOR UPDATE
	`, hash).Scan(&tokenID, &customerID, &groupID, &displayName, &tagsRaw, &expiresAt, &consumedAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Enrolled{}, ErrTokenInvalid
	}
	if err != nil {
		return Enrolled{}, fmt.Errorf("enrollment: load token: %w", err)
	}
	if consumedAt != nil || revokedAt != nil || time.Now().UTC().After(expiresAt) {
		return Enrolled{}, ErrTokenInvalid
	}

	var existing uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM devices WHERE install_id = $1`, req.InstallID).Scan(&existing)
	if err == nil {
		return Enrolled{}, ErrInstallExists
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Enrolled{}, fmt.Errorf("enrollment: check install id: %w", err)
	}

	name := req.Hostname
	if displayName != nil && *displayName != "" {
		name = *displayName
	}
	if name == "" {
		name = req.InstallID.String()
	}

	var deviceID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO devices (install_id, customer_id, group_id, display_name, hostname, agent_version, status, credential_status, tags)
		VALUES ($1,$2,$3,$4,$5,$6,'unknown','active', COALESCE($7::jsonb, '[]'::jsonb))
		RETURNING id
	`, req.InstallID, customerID, groupID, name, req.Hostname, req.AgentVersion, tagsRaw).Scan(&deviceID)
	if err != nil {
		return Enrolled{}, fmt.Errorf("enrollment: create device: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO device_credentials (device_id, credential_type, public_key, key_version)
		VALUES ($1, 'ed25519', $2, 1)
	`, deviceID, req.PublicKey)
	if err != nil {
		return Enrolled{}, fmt.Errorf("enrollment: create credential: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE enrollment_tokens SET consumed_at = now(), consumed_device_id = $2 WHERE id = $1
	`, tokenID, deviceID)
	if err != nil {
		return Enrolled{}, fmt.Errorf("enrollment: consume token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Enrolled{}, fmt.Errorf("enrollment: commit: %w", err)
	}
	return Enrolled{DeviceID: deviceID}, nil
}
