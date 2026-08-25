// Package remotesession implements remote session lifecycle (docs/PROTOCOL.md
// §10, docs/STATE_MACHINES.md §3) and temporary privilege elevation
// (docs/SPECIFICATION.md §14, docs/STATE_MACHINES.md §4) on top of the
// authenticated control channel in internal/controlhub.
package remotesession

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"wartungsremote/internal/audit"
	"wartungsremote/internal/controlhub"
	"wartungsremote/internal/maintenance"
	"wartungsremote/internal/protocol"
)

var (
	ErrNotFound     = errors.New("remotesession: not found")
	ErrDeviceOffline = errors.New("remotesession: device not connected")
	ErrAgentRejected = errors.New("remotesession: agent rejected session")
	ErrNotActive    = errors.New("remotesession: session is not active")
)

type State string

const (
	StateRequested   State = "requested"
	StateOpening     State = "opening"
	StateActive      State = "active"
	StateClosing     State = "closing"
	StateClosed      State = "closed"
	StateFailed      State = "failed"
	StateInterrupted State = "interrupted"
	StateExpired     State = "expired"
)

type Kind string

const KindTerminal Kind = "terminal"

type Session struct {
	ID                  uuid.UUID
	DeviceID            uuid.UUID
	UserID              uuid.UUID
	Kind                Kind
	State               State
	OpenedAt            time.Time
	ClosedAt            *time.Time
	ExpiresAt           time.Time
	CloseReason         string
	MaintenanceSessionID *uuid.UUID
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) Create(ctx context.Context, deviceID, userID uuid.UUID, kind Kind, expiresAt time.Time) (Session, error) {
	var s Session
	s.DeviceID, s.UserID, s.Kind, s.ExpiresAt = deviceID, userID, kind, expiresAt
	err := r.pool.QueryRow(ctx, `
		INSERT INTO remote_sessions (device_id, user_id, kind, state, expires_at)
		VALUES ($1, $2, $3, 'requested', $4)
		RETURNING id, state, opened_at
	`, deviceID, userID, string(kind), expiresAt).Scan(&s.ID, &s.State, &s.OpenedAt)
	if err != nil {
		return Session{}, fmt.Errorf("remotesession: create: %w", err)
	}
	return s, nil
}

func (r *Repo) Get(ctx context.Context, id uuid.UUID) (Session, error) {
	var s Session
	var closeReason *string
	err := r.pool.QueryRow(ctx, `
		SELECT id, device_id, user_id, kind, state, opened_at, closed_at, expires_at, close_reason, maintenance_session_id
		FROM remote_sessions WHERE id = $1
	`, id).Scan(&s.ID, &s.DeviceID, &s.UserID, &s.Kind, &s.State, &s.OpenedAt, &s.ClosedAt, &s.ExpiresAt, &closeReason, &s.MaintenanceSessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("remotesession: get: %w", err)
	}
	if closeReason != nil {
		s.CloseReason = *closeReason
	}
	return s, nil
}

func (r *Repo) SetMaintenanceSession(ctx context.Context, remoteSessionID, maintenanceSessionID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE remote_sessions SET maintenance_session_id = $2 WHERE id = $1`, remoteSessionID, maintenanceSessionID)
	if err != nil {
		return fmt.Errorf("remotesession: set maintenance session: %w", err)
	}
	return nil
}

func (r *Repo) SetState(ctx context.Context, id uuid.UUID, state State) error {
	_, err := r.pool.Exec(ctx, `UPDATE remote_sessions SET state = $2 WHERE id = $1`, id, string(state))
	if err != nil {
		return fmt.Errorf("remotesession: set state: %w", err)
	}
	return nil
}

func (r *Repo) Close(ctx context.Context, id uuid.UUID, state State, reason string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE remote_sessions SET state = $2, closed_at = now(), close_reason = $3
		WHERE id = $1
	`, id, string(state), reason)
	if err != nil {
		return fmt.Errorf("remotesession: close: %w", err)
	}
	return nil
}

// InterruptedSession identifies a session that just transitioned to
// INTERRUPTED, plus its linked maintenance session (if any) so the caller
// can close that too instead of leaving it stuck "in progress" forever.
type InterruptedSession struct {
	ID                   uuid.UUID
	MaintenanceSessionID *uuid.UUID
}

// InterruptAllForDevice marks every ACTIVE session for a device as
// INTERRUPTED, e.g. on agent disconnect (docs/STATE_MACHINES.md §3: "Keine
// INTERRUPTED Session wird stillschweigend wieder ACTIVE").
func (r *Repo) InterruptAllForDevice(ctx context.Context, deviceID uuid.UUID) ([]InterruptedSession, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE remote_sessions SET state = 'interrupted', closed_at = now(), close_reason = 'agent_disconnected'
		WHERE device_id = $1 AND state IN ('requested','opening','active')
		RETURNING id, maintenance_session_id
	`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("remotesession: interrupt all: %w", err)
	}
	defer rows.Close()
	var out []InterruptedSession
	for rows.Next() {
		var s InterruptedSession
		if err := rows.Scan(&s.ID, &s.MaintenanceSessionID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// InterruptAllActive marks every still-active session as interrupted,
// regardless of device. Called once at server startup (docs/SPECIFICATION.md
// §23 "Sessions nach Neustart als interrupted markieren") since a
// server restart drops every control-channel connection, and no
// per-device disconnect event will ever fire for sessions that were
// active at the moment of shutdown.
func (r *Repo) InterruptAllActive(ctx context.Context) ([]InterruptedSession, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE remote_sessions SET state = 'interrupted', closed_at = now(), close_reason = 'server_restart'
		WHERE state IN ('requested','opening','active')
		RETURNING id, maintenance_session_id
	`)
	if err != nil {
		return nil, fmt.Errorf("remotesession: interrupt all active: %w", err)
	}
	defer rows.Close()
	var out []InterruptedSession
	for rows.Next() {
		var s InterruptedSession
		if err := rows.Scan(&s.ID, &s.MaintenanceSessionID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Service orchestrates opening/closing sessions over the control channel.
type Service struct {
	repo        *Repo
	hub         *controlhub.Hub
	maintenance *maintenance.Repo
	audit       *audit.Logger
}

func NewService(repo *Repo, hub *controlhub.Hub, maintenanceRepo *maintenance.Repo, auditLogger *audit.Logger) *Service {
	return &Service{repo: repo, hub: hub, maintenance: maintenanceRepo, audit: auditLogger}
}

const openTimeout = 15 * time.Second

// OpenTerminal creates a remote session and asks the agent to open a
// terminal, per docs/PROTOCOL.md §10-11. It returns once the agent has
// confirmed (or rejected) the session. It also opens a maintenance session
// per docs/SPECIFICATION.md §19 (auto-start on first remote activity).
func (s *Service) OpenTerminal(ctx context.Context, deviceID, userID uuid.UUID, customerID *uuid.UUID, ttl time.Duration) (Session, error) {
	sess, err := s.repo.Create(ctx, deviceID, userID, KindTerminal, time.Now().UTC().Add(ttl))
	if err != nil {
		return Session{}, err
	}

	if s.maintenance != nil {
		if maintID, merr := s.maintenance.Open(ctx, deviceID, customerID, userID); merr == nil {
			_ = s.repo.SetMaintenanceSession(ctx, sess.ID, maintID)
			_ = s.maintenance.AddEvent(ctx, maintID, "terminal_opened", "Remote terminal session opened", &sess.ID)
			sess.MaintenanceSessionID = &maintID
		}
	}

	env, err := s.hub.SendAndWait(ctx, deviceID, protocol.TypeSessionOpen, protocol.SessionOpenPayload{
		SessionID:  sess.ID.String(),
		Kind:       string(KindTerminal),
		ExpiresAt:  sess.ExpiresAt,
		Privileged: false,
		Options:    map[string]any{"shell": "default"},
	}, openTimeout)
	if err != nil {
		_ = s.repo.Close(ctx, sess.ID, StateFailed, "agent_unreachable")
		return Session{}, ErrDeviceOffline
	}

	var result protocol.SessionOpenResultPayload
	if decErr := protocol.DecodePayload(env, &result); decErr != nil || result.Status != "success" {
		_ = s.repo.Close(ctx, sess.ID, StateFailed, "agent_rejected")
		msg := result.Message
		if msg == "" {
			msg = "no reason given by agent"
		}
		return Session{}, fmt.Errorf("%w: %s", ErrAgentRejected, msg)
	}

	if err := s.repo.SetState(ctx, sess.ID, StateActive); err != nil {
		return Session{}, err
	}
	sess.State = StateActive
	return sess, nil
}

// Close ends a session both server-side and (best effort) on the agent, and
// closes the associated maintenance session.
func (s *Service) Close(ctx context.Context, sess Session, reason string) error {
	_ = s.hub.SendMessage(ctx, sess.DeviceID, protocol.TypeSessionClose, protocol.SessionClosePayload{
		SessionID: sess.ID.String(),
		Reason:    reason,
	})
	if s.maintenance != nil && sess.MaintenanceSessionID != nil {
		_ = s.maintenance.AddEvent(ctx, *sess.MaintenanceSessionID, "terminal_closed", "Remote terminal session closed: "+reason, &sess.ID)
		_ = s.maintenance.Close(ctx, *sess.MaintenanceSessionID, "completed", "Terminal session closed: "+reason)
	}
	return s.repo.Close(ctx, sess.ID, StateClosed, reason)
}

// Resize forwards a terminal resize to the agent.
func (s *Service) Resize(ctx context.Context, sess Session, cols, rows int) error {
	return s.hub.SendMessage(ctx, sess.DeviceID, protocol.TypeTerminalResize, protocol.TerminalResizePayload{
		SessionID: sess.ID.String(),
		Cols:      cols,
		Rows:      rows,
	})
}
