// Package agentrelease manages the agent_versions manifest (docs/AGENT.md
// §15, docs/DATABASE.md agent_versions, docs/API.md §18). Release artifacts
// are signed offline with cmd/wr-release-sign; this package only verifies
// a submission against the server's configured trusted public key before
// accepting it — it never signs anything itself.
package agentrelease

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"wartungsremote/internal/agentupdate"
)

var ErrNotFound = errors.New("agentrelease: not found")

type Release struct {
	ID                uuid.UUID
	Version           string
	OSFamily          string
	Architecture      string
	Channel           string
	ArtifactURL       string
	ArtifactSHA256Hex string
	SignatureBase64   string
	PublishedAt       time.Time
	MinimumSupported  bool
	Blocked           bool
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) List(ctx context.Context) ([]Release, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, version, os_family, architecture, channel, artifact_url, artifact_sha256, signature, published_at, minimum_supported, blocked
		FROM agent_versions ORDER BY published_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("agentrelease: list: %w", err)
	}
	defer rows.Close()

	out := []Release{}
	for rows.Next() {
		rl, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rl)
	}
	return out, rows.Err()
}

// Create verifies rl's signature against trustedPubKey before inserting —
// the server never accepts a release manifest entry it can't itself prove
// was signed by the trusted offline key, even though the artifact bytes
// themselves aren't available to it (only their claimed hash).
func (r *Repo) Create(ctx context.Context, rl Release, trustedPubKey ed25519.PublicKey) (Release, error) {
	sum, err := hex.DecodeString(rl.ArtifactSHA256Hex)
	if err != nil || len(sum) != 32 {
		return Release{}, fmt.Errorf("agentrelease: invalid artifact_sha256: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(rl.SignatureBase64)
	if err != nil {
		return Release{}, fmt.Errorf("agentrelease: invalid signature encoding: %w", err)
	}
	if err := agentupdate.VerifyHashAndSignature(trustedPubKey, sum, sig); err != nil {
		return Release{}, fmt.Errorf("agentrelease: signature verification failed: %w", err)
	}

	err = r.pool.QueryRow(ctx, `
		INSERT INTO agent_versions (version, os_family, architecture, channel, artifact_url, artifact_sha256, signature, minimum_supported)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, published_at
	`, rl.Version, rl.OSFamily, rl.Architecture, rl.Channel, rl.ArtifactURL, sum, sig, rl.MinimumSupported).Scan(&rl.ID, &rl.PublishedAt)
	if err != nil {
		return Release{}, fmt.Errorf("agentrelease: create: %w", err)
	}
	return rl, nil
}

// Latest returns the most recently published, non-blocked release matching
// osFamily/architecture/channel — the candidate a device.update command
// would deploy.
func (r *Repo) Latest(ctx context.Context, osFamily, architecture, channel string) (Release, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, version, os_family, architecture, channel, artifact_url, artifact_sha256, signature, published_at, minimum_supported, blocked
		FROM agent_versions
		WHERE os_family = $1 AND architecture = $2 AND channel = $3 AND NOT blocked
		ORDER BY published_at DESC LIMIT 1
	`, osFamily, architecture, channel)
	rl, err := scanRelease(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Release{}, ErrNotFound
	}
	if err != nil {
		return Release{}, err
	}
	return rl, nil
}

// IsVersionBlocked reports whether an agent self-reporting this exact
// (osFamily, architecture, version) should be refused at the control
// channel handshake (docs/SECURITY.md §20 "Agent-Version als blockiert
// markieren"). A version with no matching manifest entry at all is never
// blocked by this check — blocking is an explicit admin action on a known
// release, not an allowlist.
func (r *Repo) IsVersionBlocked(ctx context.Context, osFamily, architecture, version string) (bool, error) {
	var blocked bool
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(bool_or(blocked), false) FROM agent_versions
		WHERE os_family = $1 AND architecture = $2 AND version = $3
	`, osFamily, architecture, version).Scan(&blocked)
	if err != nil {
		return false, fmt.Errorf("agentrelease: is version blocked: %w", err)
	}
	return blocked, nil
}

func (r *Repo) SetBlocked(ctx context.Context, id uuid.UUID, blocked bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE agent_versions SET blocked = $2 WHERE id = $1`, id, blocked)
	if err != nil {
		return fmt.Errorf("agentrelease: set blocked: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type row interface {
	Scan(dest ...any) error
}

func scanRelease(rw row) (Release, error) {
	var rl Release
	var sum, sig []byte
	if err := rw.Scan(&rl.ID, &rl.Version, &rl.OSFamily, &rl.Architecture, &rl.Channel, &rl.ArtifactURL, &sum, &sig, &rl.PublishedAt, &rl.MinimumSupported, &rl.Blocked); err != nil {
		return Release{}, fmt.Errorf("agentrelease: scan: %w", err)
	}
	rl.ArtifactSHA256Hex = hex.EncodeToString(sum)
	rl.SignatureBase64 = base64.StdEncoding.EncodeToString(sig)
	return rl, nil
}
