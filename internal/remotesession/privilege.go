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
	"wartungsremote/internal/protocol"
)

var ErrPrivilegeNotActive = errors.New("remotesession: no active privilege session")

type PrivilegeSession struct {
	ID              uuid.UUID
	RemoteSessionID uuid.UUID
	UserID          uuid.UUID
	DeviceID        uuid.UUID
	CreatedAt       time.Time
	ValidUntil      time.Time
	Reason          string
}

type PrivilegeRepo struct {
	pool *pgxpool.Pool
}

func NewPrivilegeRepo(pool *pgxpool.Pool) *PrivilegeRepo {
	return &PrivilegeRepo{pool: pool}
}

func (r *PrivilegeRepo) Create(ctx context.Context, remoteSessionID, userID, deviceID uuid.UUID, validUntil time.Time, reason string) (PrivilegeSession, error) {
	var p PrivilegeSession
	p.RemoteSessionID, p.UserID, p.DeviceID, p.ValidUntil, p.Reason = remoteSessionID, userID, deviceID, validUntil, reason
	err := r.pool.QueryRow(ctx, `
		INSERT INTO privilege_sessions (remote_session_id, user_id, device_id, valid_until, authorization_reason)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, created_at
	`, remoteSessionID, userID, deviceID, validUntil, reason).Scan(&p.ID, &p.CreatedAt)
	if err != nil {
		return PrivilegeSession{}, fmt.Errorf("remotesession: create privilege session: %w", err)
	}
	return p, nil
}

// ActiveForSession returns the currently-granted (non-revoked, non-expired)
// privilege session for a remote session, if any.
func (r *PrivilegeRepo) ActiveForSession(ctx context.Context, remoteSessionID uuid.UUID) (*PrivilegeSession, error) {
	var p PrivilegeSession
	var reason *string
	err := r.pool.QueryRow(ctx, `
		SELECT id, remote_session_id, user_id, device_id, created_at, valid_until, authorization_reason
		FROM privilege_sessions
		WHERE remote_session_id = $1 AND revoked_at IS NULL AND valid_until > now()
		ORDER BY created_at DESC LIMIT 1
	`, remoteSessionID).Scan(&p.ID, &p.RemoteSessionID, &p.UserID, &p.DeviceID, &p.CreatedAt, &p.ValidUntil, &reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("remotesession: active privilege: %w", err)
	}
	if reason != nil {
		p.Reason = *reason
	}
	return &p, nil
}

func (r *PrivilegeRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE privilege_sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("remotesession: revoke privilege: %w", err)
	}
	return nil
}

// expiredButNotRevoked finds privilege sessions whose valid_until has
// passed but that haven't been marked revoked yet, so the sweeper can
// notify the agent and stamp revoked_at.
func (r *PrivilegeRepo) expiredButNotRevoked(ctx context.Context) ([]PrivilegeSession, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, remote_session_id, user_id, device_id, created_at, valid_until
		FROM privilege_sessions
		WHERE revoked_at IS NULL AND valid_until <= now()
	`)
	if err != nil {
		return nil, fmt.Errorf("remotesession: query expired privileges: %w", err)
	}
	defer rows.Close()
	var out []PrivilegeSession
	for rows.Next() {
		var p PrivilegeSession
		if err := rows.Scan(&p.ID, &p.RemoteSessionID, &p.UserID, &p.DeviceID, &p.CreatedAt, &p.ValidUntil); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GrantPrivilege creates a privilege session bound to an ACTIVE remote
// session and immediately notifies the agent, per docs/SPECIFICATION.md §14.
// Callers MUST have already re-verified password+TOTP (reauth) before
// calling this — see internal/auth.Service.ConsumeReauth.
func (s *Service) GrantPrivilege(ctx context.Context, privRepo *PrivilegeRepo, sess Session, duration, maxDuration time.Duration, reason string) (PrivilegeSession, error) {
	if sess.State != StateActive {
		return PrivilegeSession{}, ErrNotActive
	}
	if duration <= 0 || duration > maxDuration {
		duration = maxDuration
	}
	validUntil := time.Now().UTC().Add(duration)

	p, err := privRepo.Create(ctx, sess.ID, sess.UserID, sess.DeviceID, validUntil, reason)
	if err != nil {
		return PrivilegeSession{}, err
	}

	_ = s.hub.SendMessage(ctx, sess.DeviceID, protocol.TypeSessionPrivilegeUpdate, protocol.SessionPrivilegeUpdatePayload{
		SessionID:       sess.ID.String(),
		Privileged:      true,
		ValidUntil:      validUntil,
		AuthorizationID: p.ID.String(),
	})
	return p, nil
}

// RevokePrivilege immediately withdraws elevated rights, either by explicit
// admin action or from the expiry sweeper.
func (s *Service) RevokePrivilege(ctx context.Context, privRepo *PrivilegeRepo, p PrivilegeSession) error {
	if err := privRepo.Revoke(ctx, p.ID); err != nil {
		return err
	}
	_ = s.hub.SendMessage(ctx, p.DeviceID, protocol.TypeSessionPrivilegeUpdate, protocol.SessionPrivilegeUpdatePayload{
		SessionID:  p.RemoteSessionID.String(),
		Privileged: false,
	})
	return nil
}

// RunPrivilegeExpirySweeper periodically revokes privilege sessions whose
// valid_until has passed, so elevated rights are withdrawn automatically
// even without further admin/browser activity (docs/SPECIFICATION.md §14
// step 7).
func (s *Service) RunPrivilegeExpirySweeper(ctx context.Context, privRepo *PrivilegeRepo) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expired, err := privRepo.expiredButNotRevoked(ctx)
			if err != nil {
				continue
			}
			for _, p := range expired {
				if err := s.RevokePrivilege(ctx, privRepo, p); err != nil {
					continue
				}
				// The explicit DELETE /sessions/:id/privilege path audits
				// itself (it has request context like source IP); automatic
				// expiry has no such request, but docs/SPECIFICATION.md §20
				// still requires "Privilege ... Ende" to be audited, so it's
				// recorded here instead.
				if s.audit != nil {
					_ = s.audit.Record(ctx, audit.Event{
						ActorType: audit.ActorSystem,
						DeviceID:  &p.DeviceID,
						SessionID: &p.RemoteSessionID,
						EventType: "privilege.expired",
						Result:    audit.ResultSuccess,
						Metadata:  map[string]any{"privilege_session_id": p.ID},
					})
				}
			}
		}
	}
}
