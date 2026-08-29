// Package audit writes append-only audit log entries per docs/SECURITY.md §16
// and docs/SPECIFICATION.md §20. Callers MUST NOT pass secrets (passwords,
// TOTP codes, private keys, session tokens, full file contents) in Metadata;
// this package does not attempt to detect and redact such content.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// auditChainLockKey is an arbitrary fixed key for pg_advisory_xact_lock,
// used only to fully serialize audit_log inserts so the hash chain can
// never fork under concurrent writers — released automatically at
// commit/rollback, held only for the few milliseconds an insert takes.
const auditChainLockKey = 847362910183746521

// Record inserts an audit event. It never returns a permission/business
// error to the caller's request flow; callers should log+continue on
// failure rather than fail the underlying operation, but MUST NOT silently
// skip calling Record for audit-relevant actions.
//
// Every entry is chained to the previous one (prev_hash/entry_hash,
// docs/SECURITY.md §16): entry_hash = SHA256(prev_hash || this entry's
// fields). Combined with the append-only DB trigger
// (migrations/0003_audit_append_only.sql), this means an entry can never
// be silently edited or deleted after the fact without breaking the
// chain from that point forward — detectable via VerifyChain — even by
// someone with direct database access, though not by someone with the
// TOTP encryption key AND database access recomputing the whole chain
// from scratch (this defends against a partial/accidental edit, not
// every conceivable insider-threat scenario).
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

	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("audit: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(auditChainLockKey)); err != nil {
		return fmt.Errorf("audit: acquire chain lock: %w", err)
	}

	var prevHash []byte
	err = tx.QueryRow(ctx, `SELECT entry_hash FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&prevHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("audit: read previous hash: %w", err)
	}

	// Truncated to microseconds: that's all a timestamptz column stores,
	// so hashing the untruncated nanosecond-precision value here would
	// hash something different from what VerifyChain reads back later,
	// reporting every entry as tampered even when nothing touched it.
	occurredAt := time.Now().UTC().Truncate(time.Microsecond)
	entryHash := computeEntryHash(prevHash, chainableFields{
		OccurredAt: occurredAt, ActorType: string(ev.ActorType), ActorID: ev.ActorID,
		SessionID: ev.SessionID, DeviceID: ev.DeviceID, CustomerID: ev.CustomerID,
		EventType: ev.EventType, Result: string(ev.Result), SourceIP: ev.SourceIP,
		RequestID: ev.RequestID, MetadataJSON: metaJSON,
	})

	_, err = tx.Exec(ctx, `
		INSERT INTO audit_log
			(occurred_at, actor_type, actor_id, session_id, device_id, customer_id,
			 event_type, result, source_ip, request_id, metadata, prev_hash, entry_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`,
		occurredAt, string(ev.ActorType), ev.ActorID, ev.SessionID, ev.DeviceID, ev.CustomerID,
		ev.EventType, string(ev.Result), sourceIP, ev.RequestID, metaJSON, prevHash, entryHash,
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}
	return tx.Commit(ctx)
}

// chainableFields is exactly what goes into an entry's hash — kept as one
// struct so Record (writing) and VerifyChain (reading back) can never
// drift out of sync about which fields are covered.
type chainableFields struct {
	OccurredAt   time.Time
	ActorType    string
	ActorID      *uuid.UUID
	SessionID    *uuid.UUID
	DeviceID     *uuid.UUID
	CustomerID   *uuid.UUID
	EventType    string
	Result       string
	SourceIP     string
	RequestID    *uuid.UUID
	MetadataJSON []byte
}

func writeUUIDPtr(h interface{ Write([]byte) (int, error) }, id *uuid.UUID) {
	if id != nil {
		_, _ = h.Write([]byte(id.String()))
	}
}

func computeEntryHash(prevHash []byte, f chainableFields) []byte {
	h := sha256.New()
	_, _ = h.Write(prevHash)
	_, _ = h.Write([]byte(f.OccurredAt.Format(time.RFC3339Nano)))
	_, _ = h.Write([]byte(f.ActorType))
	writeUUIDPtr(h, f.ActorID)
	writeUUIDPtr(h, f.SessionID)
	writeUUIDPtr(h, f.DeviceID)
	writeUUIDPtr(h, f.CustomerID)
	_, _ = h.Write([]byte(f.EventType))
	_, _ = h.Write([]byte(f.Result))
	_, _ = h.Write([]byte(f.SourceIP))
	writeUUIDPtr(h, f.RequestID)
	_, _ = h.Write(f.MetadataJSON)
	return h.Sum(nil)
}

// ChainVerification is the result of walking the whole audit_log hash
// chain from the beginning.
type ChainVerification struct {
	Valid        bool
	EntriesCheck int64
	// EntriesPreChain counts leading rows (by id) that predate this
	// feature — inserted before Record() started populating entry_hash,
	// so they carry no hash to verify. Not a sign of tampering, just an
	// older install; excluded from EntriesCheck.
	EntriesPreChain int64
	// BrokenAtID is the id of the first entry whose stored hash doesn't
	// match what its fields recompute to, or whose prev_hash doesn't
	// match the previous entry's entry_hash — i.e. the earliest point
	// where the chain no longer proves it's unmodified from there back.
	BrokenAtID *int64
}

// VerifyChain recomputes every entry's hash from its stored fields and
// checks it against what's actually stored, in id order — the whole
// point of the chain (docs/SECURITY.md §16): this can only ever get to
// "valid" if every single row since the chain started has its original
// content. Safe to run at any time; read-only, no lock held beyond each
// row's own read.
//
// Rows inserted before this feature shipped have entry_hash = NULL —
// they were never part of any chain, so a leading run of NULL-hash rows
// is treated as pre-chain history, not a broken link. Once the first
// non-NULL entry_hash is seen, chain verification starts there — which
// naturally expects prev_hash = NULL on that row too, since Record()
// read it back off the last (pre-chain) row at the time. A NULL
// entry_hash appearing *after* the chain has already started is a real
// break (something inserted that bypassed Record), not pre-chain
// history, and is reported like any other mismatch.
func (l *Logger) VerifyChain(ctx context.Context) (ChainVerification, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT id, occurred_at, actor_type, actor_id, session_id, device_id, customer_id,
		       event_type, result, host(source_ip), request_id, metadata, prev_hash, entry_hash
		FROM audit_log ORDER BY id ASC
	`)
	if err != nil {
		return ChainVerification{}, fmt.Errorf("audit: verify chain: %w", err)
	}
	defer rows.Close()

	var expectedPrev []byte
	var count, preChain int64
	chainStarted := false
	for rows.Next() {
		var id int64
		var occurredAt time.Time
		var actorType, eventType, result string
		var actorID, sessionID, deviceID, customerID, requestID *uuid.UUID
		var sourceIP *string
		var metaRaw, prevHash, entryHash []byte
		if err := rows.Scan(&id, &occurredAt, &actorType, &actorID, &sessionID, &deviceID, &customerID,
			&eventType, &result, &sourceIP, &requestID, &metaRaw, &prevHash, &entryHash); err != nil {
			return ChainVerification{}, fmt.Errorf("audit: scan for verify: %w", err)
		}

		if !chainStarted && len(entryHash) == 0 {
			preChain++
			continue
		}
		chainStarted = true
		count++

		// Re-marshal through Go's encoding/json rather than trusting
		// Postgres's stored jsonb text byte-for-byte — this reproduces
		// exactly what Record() originally hashed regardless of how
		// jsonb chose to store it internally (both are canonical Go
		// json.Marshal output of the same logical map).
		var metaMap map[string]any
		if len(metaRaw) > 0 {
			if err := json.Unmarshal(metaRaw, &metaMap); err != nil {
				return ChainVerification{}, fmt.Errorf("audit: unmarshal stored metadata for id %d: %w", id, err)
			}
		}
		metaJSON, err := json.Marshal(metaMap)
		if err != nil {
			return ChainVerification{}, fmt.Errorf("audit: re-marshal metadata for id %d: %w", id, err)
		}

		var ip string
		if sourceIP != nil {
			ip = *sourceIP
		}
		// pgx scans timestamptz back in the local zone, not UTC — same
		// instant, but a different RFC3339Nano string (e.g. "+02:00" vs
		// "Z"). Record always hashes the UTC-formatted string, so this
		// must match that or every entry looks tampered.
		recomputed := computeEntryHash(expectedPrev, chainableFields{
			OccurredAt: occurredAt.UTC(), ActorType: actorType, ActorID: actorID,
			SessionID: sessionID, DeviceID: deviceID, CustomerID: customerID,
			EventType: eventType, Result: result, SourceIP: ip,
			RequestID: requestID, MetadataJSON: metaJSON,
		})

		brokenID := id
		if !bytesEqual(prevHash, expectedPrev) || !bytesEqual(entryHash, recomputed) {
			return ChainVerification{Valid: false, EntriesCheck: count, EntriesPreChain: preChain, BrokenAtID: &brokenID}, nil
		}
		expectedPrev = entryHash
	}
	if err := rows.Err(); err != nil {
		return ChainVerification{}, fmt.Errorf("audit: verify chain: %w", err)
	}
	return ChainVerification{Valid: true, EntriesCheck: count, EntriesPreChain: preChain}, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
