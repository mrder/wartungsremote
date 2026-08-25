package remotesession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"wartungsremote/internal/protocol"
)

// Additional Kind values for SSH/RDP tunnels, per docs/DATABASE.md
// remote_sessions.kind and docs/SPECIFICATION.md §16.
const (
	KindSSHTunnel Kind = "ssh_tunnel"
	KindRDPTunnel Kind = "rdp_tunnel"
)

// TargetType is the semantic tunnel destination the agent resolves locally
// — never a host/port taken from request input (docs/SECURITY.md §12).
type TargetType string

const (
	TargetSSHLocal TargetType = "ssh_local"
	TargetRDPLocal TargetType = "rdp_local"
)

var (
	ErrInvalidTargetType = errors.New("remotesession: invalid tunnel target_type")
	ErrTicketInvalid     = errors.New("remotesession: tunnel ticket invalid, expired, or already used")
)

func kindForTarget(t TargetType) (Kind, byte, error) {
	switch t {
	case TargetSSHLocal:
		return KindSSHTunnel, protocol.StreamKindTunnel, nil
	case TargetRDPLocal:
		return KindRDPTunnel, protocol.StreamKindTunnel, nil
	default:
		return "", 0, ErrInvalidTargetType
	}
}

type TunnelSession struct {
	ID              uuid.UUID
	RemoteSessionID uuid.UUID
	DeviceID        uuid.UUID
	UserID          uuid.UUID
	TargetType      TargetType
	State           string
	ExpiresAt       time.Time
}

type TunnelRepo struct {
	pool *pgxpool.Pool
}

func NewTunnelRepo(pool *pgxpool.Pool) *TunnelRepo {
	return &TunnelRepo{pool: pool}
}

// ticketEntropyBytes: >= 256 bit per docs/RELAY.md §4.
const ticketEntropyBytes = 32

func generateTicket() (plaintext string, hash []byte, err error) {
	raw := make([]byte, ticketEntropyBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("remotesession: generate ticket: %w", err)
	}
	plaintext = "wr_tunnel_" + base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256(raw)
	return plaintext, sum[:], nil
}

func hashTicket(ticket string) ([]byte, error) {
	const prefix = "wr_tunnel_"
	if len(ticket) <= len(prefix) || ticket[:len(prefix)] != prefix {
		return nil, ErrTicketInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(ticket[len(prefix):])
	if err != nil {
		return nil, ErrTicketInvalid
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}

func (r *TunnelRepo) create(ctx context.Context, remoteSessionID, deviceID, userID uuid.UUID, targetType TargetType, ticketHash []byte, expiresAt time.Time) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO tunnel_sessions (remote_session_id, device_id, user_id, target_type, state, helper_ticket_hash, expires_at)
		VALUES ($1,$2,$3,$4,'ticket_issued',$5,$6)
		RETURNING id
	`, remoteSessionID, deviceID, userID, string(targetType), ticketHash, expiresAt).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("remotesession: create tunnel: %w", err)
	}
	return id, nil
}

// ConsumeTicket atomically validates and single-use-consumes a helper
// ticket, per docs/RELAY.md §4. Uses SELECT ... FOR UPDATE to serialize
// concurrent redemption attempts the same way enrollment tokens are
// consumed (internal/enrollment.Service.Consume).
func (r *TunnelRepo) ConsumeTicket(ctx context.Context, ticket string) (TunnelSession, error) {
	hash, err := hashTicket(ticket)
	if err != nil {
		return TunnelSession{}, err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TunnelSession{}, fmt.Errorf("remotesession: begin ticket consume: %w", err)
	}
	defer tx.Rollback(ctx)

	var t TunnelSession
	var state string
	err = tx.QueryRow(ctx, `
		SELECT id, remote_session_id, device_id, user_id, target_type, state, expires_at
		FROM tunnel_sessions WHERE helper_ticket_hash = $1 FOR UPDATE
	`, hash).Scan(&t.ID, &t.RemoteSessionID, &t.DeviceID, &t.UserID, &t.TargetType, &state, &t.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return TunnelSession{}, ErrTicketInvalid
	}
	if err != nil {
		return TunnelSession{}, fmt.Errorf("remotesession: load ticket: %w", err)
	}
	if state != "ticket_issued" || time.Now().UTC().After(t.ExpiresAt) {
		return TunnelSession{}, ErrTicketInvalid
	}

	if _, err := tx.Exec(ctx, `UPDATE tunnel_sessions SET state = 'active', connected_at = now() WHERE id = $1`, t.ID); err != nil {
		return TunnelSession{}, fmt.Errorf("remotesession: consume ticket: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TunnelSession{}, fmt.Errorf("remotesession: commit ticket consume: %w", err)
	}
	t.State = "active"
	return t, nil
}

// UpdateBytes records transfer volume for a tunnel (docs/RELAY.md §9: byte
// counts are optional but, when tracked, must never include the SSH/RDP
// payload contents themselves — only aggregate sizes).
func (r *TunnelRepo) UpdateBytes(ctx context.Context, id uuid.UUID, up, down int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE tunnel_sessions SET bytes_up = $2, bytes_down = $3 WHERE id = $1`, id, up, down)
	if err != nil {
		return fmt.Errorf("remotesession: update tunnel bytes: %w", err)
	}
	return nil
}

func (r *TunnelRepo) Close(ctx context.Context, id uuid.UUID, state string) error {
	_, err := r.pool.Exec(ctx, `UPDATE tunnel_sessions SET state = $2, closed_at = now() WHERE id = $1`, id, state)
	if err != nil {
		return fmt.Errorf("remotesession: close tunnel: %w", err)
	}
	return nil
}

// InvalidateAllOutstanding revokes every not-yet-used tunnel ticket, e.g. on
// server restart (docs/RELAY.md §8: "kann V1 alle outstanding Tickets beim
// Relay/Core-Neustart invalidieren").
func (r *TunnelRepo) InvalidateAllOutstanding(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE tunnel_sessions SET state = 'expired', closed_at = now()
		WHERE state = 'ticket_issued'
	`)
	if err != nil {
		return fmt.Errorf("remotesession: invalidate outstanding tickets: %w", err)
	}
	return nil
}

const tunnelOpenTimeout = 15 * time.Second

// OpenTunnel creates a remote session + single-use helper ticket for an
// SSH/RDP tunnel, per docs/SPECIFICATION.md §16 and docs/RELAY.md §5. The
// agent is asked to validate + prepare the target immediately (same
// session_open/session_open_result handshake as terminals); the ticket
// itself is what a later wr-helper connection redeems to attach the actual
// byte stream.
func (s *Service) OpenTunnel(ctx context.Context, tunnelRepo *TunnelRepo, deviceID, userID uuid.UUID, targetType TargetType, sessionTTL, ticketTTL time.Duration) (Session, string, uuid.UUID, error) {
	kind, _, err := kindForTarget(targetType)
	if err != nil {
		return Session{}, "", uuid.Nil, err
	}

	sess, err := s.repo.Create(ctx, deviceID, userID, kind, time.Now().UTC().Add(sessionTTL))
	if err != nil {
		return Session{}, "", uuid.Nil, err
	}

	env, err := s.hub.SendAndWait(ctx, deviceID, protocol.TypeSessionOpen, protocol.SessionOpenPayload{
		SessionID:  sess.ID.String(),
		Kind:       string(targetType),
		ExpiresAt:  sess.ExpiresAt,
		Privileged: false,
	}, tunnelOpenTimeout)
	if err != nil {
		_ = s.repo.Close(ctx, sess.ID, StateFailed, "agent_unreachable")
		return Session{}, "", uuid.Nil, ErrDeviceOffline
	}
	var result protocol.SessionOpenResultPayload
	if decErr := protocol.DecodePayload(env, &result); decErr != nil || result.Status != "success" {
		_ = s.repo.Close(ctx, sess.ID, StateFailed, "agent_rejected")
		msg := result.Message
		if msg == "" {
			msg = "no reason given by agent"
		}
		return Session{}, "", uuid.Nil, fmt.Errorf("%w: %s", ErrAgentRejected, msg)
	}
	if err := s.repo.SetState(ctx, sess.ID, StateActive); err != nil {
		return Session{}, "", uuid.Nil, err
	}
	sess.State = StateActive

	ticket, hash, err := generateTicket()
	if err != nil {
		return Session{}, "", uuid.Nil, err
	}
	tunnelID, err := tunnelRepo.create(ctx, sess.ID, deviceID, userID, targetType, hash, time.Now().UTC().Add(ticketTTL))
	if err != nil {
		return Session{}, "", uuid.Nil, err
	}

	return sess, ticket, tunnelID, nil
}
