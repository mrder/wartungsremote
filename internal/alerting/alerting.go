// Package alerting implements the alert rule engine (docs/TODO.md Phase 24):
// user-configurable rules evaluated against device state, opening/resolving
// rows in the append-visible (not append-only — alerts can be acknowledged
// and resolved) `alerts` table. Notification channels (email/ntfy/Telegram/
// webhooks) are explicitly out of scope for V1 per docs/TODO.md.
package alerting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Rule types. Config JSON shape per type is documented alongside each
// evaluation branch in engine.go.
const (
	RuleOffline      = "offline"
	RuleCPU          = "cpu"
	RuleRAM          = "ram"
	RuleDisk         = "disk"
	RuleService      = "service"
	RuleAgentVersion = "agent_version"
)

const (
	ScopeGlobal   = "global"
	ScopeCustomer = "customer"
	ScopeGroup    = "group"
	ScopeDevice   = "device"
)

const (
	AlertStateOpen         = "open"
	AlertStateAcknowledged = "acknowledged"
	AlertStateResolved     = "resolved"
)

var ErrNotFound = errors.New("alerting: not found")

type Rule struct {
	ID        uuid.UUID
	ScopeType string
	ScopeID   *uuid.UUID
	RuleType  string
	Config    json.RawMessage
	Enabled   bool
}

type Alert struct {
	ID         uuid.UUID
	DeviceID   uuid.UUID
	RuleID     *uuid.UUID
	Severity   string
	State      string
	OpenedAt   time.Time
	ResolvedAt *time.Time
	Summary    string
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func (r *Repo) ListRules(ctx context.Context) ([]Rule, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, scope_type, scope_id, rule_type, config, enabled FROM alert_rules ORDER BY rule_type, scope_type`)
	if err != nil {
		return nil, fmt.Errorf("alerting: list rules: %w", err)
	}
	defer rows.Close()

	out := []Rule{}
	for rows.Next() {
		var rl Rule
		if err := rows.Scan(&rl.ID, &rl.ScopeType, &rl.ScopeID, &rl.RuleType, &rl.Config, &rl.Enabled); err != nil {
			return nil, fmt.Errorf("alerting: scan rule: %w", err)
		}
		out = append(out, rl)
	}
	return out, rows.Err()
}

func (r *Repo) CreateRule(ctx context.Context, rl Rule) (Rule, error) {
	if rl.Config == nil {
		rl.Config = json.RawMessage(`{}`)
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO alert_rules (scope_type, scope_id, rule_type, config, enabled)
		VALUES ($1,$2,$3,$4,$5) RETURNING id
	`, rl.ScopeType, rl.ScopeID, rl.RuleType, rl.Config, rl.Enabled).Scan(&rl.ID)
	if err != nil {
		return Rule{}, fmt.Errorf("alerting: create rule: %w", err)
	}
	return rl, nil
}

// SetRuleEnabled toggles a rule on/off without needing full-object replace.
func (r *Repo) SetRuleEnabled(ctx context.Context, id uuid.UUID, enabled bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE alert_rules SET enabled = $2 WHERE id = $1`, id, enabled)
	if err != nil {
		return fmt.Errorf("alerting: set rule enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) DeleteRule(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("alerting: delete rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

type AlertFilter struct {
	DeviceID *uuid.UUID
	State    string
}

func (r *Repo) ListAlerts(ctx context.Context, f AlertFilter) ([]Alert, error) {
	where := "WHERE 1=1"
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.DeviceID != nil {
		where += " AND device_id = " + arg(*f.DeviceID)
	}
	if f.State != "" {
		where += " AND state = " + arg(f.State)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, device_id, rule_id, severity, state, opened_at, resolved_at, summary
		FROM alerts `+where+` ORDER BY opened_at DESC LIMIT 500
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("alerting: list alerts: %w", err)
	}
	defer rows.Close()

	out := []Alert{}
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.DeviceID, &a.RuleID, &a.Severity, &a.State, &a.OpenedAt, &a.ResolvedAt, &a.Summary); err != nil {
			return nil, fmt.Errorf("alerting: scan alert: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *Repo) CountOpen(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM alerts WHERE state IN ('open','acknowledged')`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("alerting: count open: %w", err)
	}
	return n, nil
}

// OpenIfNotExists inserts a new open alert for (deviceID, ruleID) unless one
// is already open/acknowledged, so a still-triggering condition doesn't spam
// duplicate alert rows on every evaluation tick.
func (r *Repo) OpenIfNotExists(ctx context.Context, deviceID uuid.UUID, ruleID uuid.UUID, severity, summary string) (opened bool, err error) {
	var existing uuid.UUID
	err = r.pool.QueryRow(ctx, `
		SELECT id FROM alerts WHERE device_id = $1 AND rule_id = $2 AND state IN ('open','acknowledged') LIMIT 1
	`, deviceID, ruleID).Scan(&existing)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("alerting: check existing alert: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO alerts (device_id, rule_id, severity, state, summary) VALUES ($1,$2,$3,'open',$4)
	`, deviceID, ruleID, severity, summary); err != nil {
		return false, fmt.Errorf("alerting: open alert: %w", err)
	}
	return true, nil
}

// AutoResolveIfOpen closes any open/acknowledged alert for (deviceID,
// ruleID) once the underlying condition has cleared, reporting whether it
// actually resolved a row (for audit logging).
func (r *Repo) AutoResolveIfOpen(ctx context.Context, deviceID, ruleID uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE alerts SET state='resolved', resolved_at=now()
		WHERE device_id = $1 AND rule_id = $2 AND state IN ('open','acknowledged')
	`, deviceID, ruleID)
	if err != nil {
		return false, fmt.Errorf("alerting: auto-resolve: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repo) Acknowledge(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `UPDATE alerts SET state='acknowledged' WHERE id = $1 AND state='open'`, id)
	if err != nil {
		return fmt.Errorf("alerting: acknowledge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) Resolve(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `UPDATE alerts SET state='resolved', resolved_at=now() WHERE id = $1 AND state != 'resolved'`, id)
	if err != nil {
		return fmt.Errorf("alerting: resolve: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete permanently removes an alert record. Alerts are otherwise kept
// indefinitely (no automatic retention sweep, unlike raw/hourly metrics) —
// this is the only way one goes away, and it's an explicit, audited user
// action (docs/API.md), not a background cleanup.
func (r *Repo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM alerts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("alerting: delete: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
