// Package audit writes append-only audit log entries per docs/SECURITY.md §16
// and docs/SPECIFICATION.md §20. Callers MUST NOT pass secrets (passwords,
// TOTP codes, private keys, session tokens, full file contents) in Metadata;
// this package does not attempt to detect and redact such content.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry is a stored audit_log row, as read back for the admin API.
type Entry struct {
	ID         int64
	OccurredAt time.Time
	ActorType  string
	ActorID    *uuid.UUID
	// ActorUsername is resolved via a join for display convenience — nil
	// for non-user actors (system/agent events) or a user later deleted.
	ActorUsername *string
	DeviceID      *uuid.UUID
	CustomerID    *uuid.UUID
	EventType     string
	Result        string
	SourceIP      string
	Metadata      map[string]any
}

// Filter narrows a List query. Zero values mean "no filter" for that field.
type Filter struct {
	ActorID   *uuid.UUID
	DeviceID  *uuid.UUID
	EventType string
	From, To  time.Time
	Limit     int
}

type ActorType string

const (
	ActorUser   ActorType = "user"
	ActorAgent  ActorType = "agent"
	ActorSystem ActorType = "system"
)

type Result string

const (
	ResultSuccess Result = "success"
	ResultFailure Result = "failure"
	ResultDenied  Result = "denied"
)

// Well-known event types (non-exhaustive; free-form strings are accepted so
// new event types don't require a code change here, but keep names stable
// once shipped).
const (
	EventLoginSuccess        = "auth.login.success"
	EventLoginFailure        = "auth.login.failure"
	EventLoginLockout        = "auth.login.lockout"
	EventLogout              = "auth.logout"
	EventMFAChallengeSuccess = "auth.mfa.success"
	EventMFAChallengeFailure = "auth.mfa.failure"
	EventReauthSuccess       = "auth.reauth.success"
	EventReauthFailure       = "auth.reauth.failure"
	EventEnrollmentCreated   = "enrollment.created"
	EventEnrollmentConsumed  = "enrollment.consumed"
	EventEnrollmentRejected  = "enrollment.rejected"
	EventEnrollmentRevoked   = "enrollment.revoked"
	EventDeviceUpdated       = "device.updated"
	EventDeviceRevoked       = "device.revoked"
	EventDeviceConnected     = "device.connected"
	EventDeviceDisconnected  = "device.disconnected"
	EventProtocolError       = "protocol.error"
)

// Event is a single audit log entry.
type Event struct {
	ActorType  ActorType
	ActorID    *uuid.UUID
	SessionID  *uuid.UUID
	DeviceID   *uuid.UUID
	CustomerID *uuid.UUID
	EventType  string
	Result     Result
	SourceIP   string
	RequestID  *uuid.UUID
	Metadata   map[string]any
}

// Logger records audit events.
type Logger struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Logger {
	return &Logger{pool: pool}
}

// Record inserts an audit event. It never returns a permission/business
// error to the caller's request flow; callers should log+continue on
// failure rather than fail the underlying operation, but MUST NOT silently
// skip calling Record for audit-relevant actions.
func (l *Logger) Record(ctx context.Context, ev Event) error {
	meta := ev.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("audit: marshal metadata: %w", err)
	}

	var sourceIP any
	if ev.SourceIP != "" {
		sourceIP = ev.SourceIP
	}

	_, err = l.pool.Exec(ctx, `
		INSERT INTO audit_log
			(occurred_at, actor_type, actor_id, session_id, device_id, customer_id,
			 event_type, result, source_ip, request_id, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		time.Now().UTC(), string(ev.ActorType), ev.ActorID, ev.SessionID, ev.DeviceID, ev.CustomerID,
		ev.EventType, string(ev.Result), sourceIP, ev.RequestID, metaJSON,
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return nil
}

// List returns audit entries matching f, newest first. There is
// intentionally no delete/update counterpart: the audit log is append-only
// (docs/SECURITY.md §16, enforced additionally by a DB trigger).
func (l *Logger) List(ctx context.Context, f Filter) ([]Entry, error) {
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 200
	}
	if f.To.IsZero() {
		f.To = time.Now().UTC()
	}
	if f.From.IsZero() {
		f.From = f.To.Add(-30 * 24 * time.Hour)
	}

	where := "WHERE a.occurred_at BETWEEN $1 AND $2"
	args := []any{f.From, f.To}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.ActorID != nil {
		where += " AND a.actor_id = " + arg(*f.ActorID)
	}
	if f.DeviceID != nil {
		where += " AND a.device_id = " + arg(*f.DeviceID)
	}
	if f.EventType != "" {
		where += " AND a.event_type = " + arg(f.EventType)
	}
	limitArg := arg(f.Limit)

	rows, err := l.pool.Query(ctx, `
		SELECT a.id, a.occurred_at, a.actor_type, a.actor_id, u.username, a.device_id, a.customer_id, a.event_type, a.result, host(a.source_ip), a.metadata
		FROM audit_log a
		LEFT JOIN users u ON u.id = a.actor_id
		`+where+` ORDER BY a.occurred_at DESC LIMIT `+limitArg, args...)
	if err != nil {
		return nil, fmt.Errorf("audit: list: %w", err)
	}
	defer rows.Close()

	out := []Entry{}
	for rows.Next() {
		var e Entry
		var meta []byte
		var sourceIP *string
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.ActorType, &e.ActorID, &e.ActorUsername, &e.DeviceID, &e.CustomerID, &e.EventType, &e.Result, &sourceIP, &meta); err != nil {
			return nil, fmt.Errorf("audit: scan: %w", err)
		}
		if sourceIP != nil {
			e.SourceIP = *sourceIP
		}
		_ = json.Unmarshal(meta, &e.Metadata)
		out = append(out, e)
	}
	return out, rows.Err()
}
