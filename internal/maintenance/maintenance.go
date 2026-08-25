// Package maintenance records maintenance sessions per
// docs/SPECIFICATION.md §19: every remote session is associated with a
// maintenance session, auto-started on first remote activity.
package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Session struct {
	ID        uuid.UUID
	DeviceID  uuid.UUID
	UserID    uuid.UUID
	StartedAt time.Time
	EndedAt   *time.Time
	Result    string
	Summary   string
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Open starts a new maintenance session. V1 creates one per remote session
// (see internal/remotesession), which is a simpler but spec-compliant
// reading of "automatischer Start bei erster Remote-Aktion" — grouping
// multiple remote sessions from one visit into a single maintenance record
// is left for a later phase (manual maintenance session start/merge).
func (r *Repo) Open(ctx context.Context, deviceID uuid.UUID, customerID *uuid.UUID, userID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `
		INSERT INTO maintenance_sessions (device_id, customer_id, user_id)
		VALUES ($1,$2,$3) RETURNING id
	`, deviceID, customerID, userID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("maintenance: open: %w", err)
	}
	return id, nil
}

func (r *Repo) AddEvent(ctx context.Context, maintenanceSessionID uuid.UUID, eventType, summary string, referenceID *uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO maintenance_events (maintenance_session_id, event_type, summary, reference_id)
		VALUES ($1,$2,$3,$4)
	`, maintenanceSessionID, eventType, summary, referenceID)
	if err != nil {
		return fmt.Errorf("maintenance: add event: %w", err)
	}
	return nil
}

func (r *Repo) Close(ctx context.Context, id uuid.UUID, result, summary string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE maintenance_sessions SET ended_at = now(), result = $2, summary = $3 WHERE id = $1
	`, id, result, summary)
	if err != nil {
		return fmt.Errorf("maintenance: close: %w", err)
	}
	return nil
}

func (r *Repo) ListForDevice(ctx context.Context, deviceID uuid.UUID) ([]Session, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, device_id, user_id, started_at, ended_at, COALESCE(result,''), COALESCE(summary,'')
		FROM maintenance_sessions WHERE device_id = $1 ORDER BY started_at DESC LIMIT 200
	`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("maintenance: list for device: %w", err)
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.DeviceID, &s.UserID, &s.StartedAt, &s.EndedAt, &s.Result, &s.Summary); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
