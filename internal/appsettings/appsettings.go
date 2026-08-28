// Package appsettings holds the small set of settings that are runtime-
// adjustable from the dashboard (currently just metrics retention) instead
// of fixed in server.yaml — so an operator can change them without a
// restart. Absence of a key means "use the server.yaml default", never a
// silent zero/unlimited value.
package appsettings

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	KeyMetricsRawRetentionHours    = "metrics.raw_retention_hours"
	KeyMetricsHourlyRetentionHours = "metrics.hourly_retention_hours"
)

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

// Get returns a raw setting value, or ok=false if it's never been set
// (meaning: use the server.yaml default).
func (r *Repo) Get(ctx context.Context, key string) (value string, ok bool, err error) {
	err = r.pool.QueryRow(ctx, `SELECT value FROM app_settings WHERE key = $1`, key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("appsettings: get %s: %w", key, err)
	}
	return value, true, nil
}

func (r *Repo) Set(ctx context.Context, key, value string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO app_settings (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
	`, key, value)
	if err != nil {
		return fmt.Errorf("appsettings: set %s: %w", key, err)
	}
	return nil
}

// MetricsRetention returns the effective raw/hourly retention windows:
// the stored override if one exists, otherwise the given server.yaml
// defaults.
func (r *Repo) MetricsRetention(ctx context.Context, defaultRaw, defaultHourly time.Duration) (raw, hourly time.Duration, err error) {
	raw = defaultRaw
	hourly = defaultHourly
	if v, ok, gerr := r.Get(ctx, KeyMetricsRawRetentionHours); gerr != nil {
		return 0, 0, gerr
	} else if ok {
		if h, perr := strconv.Atoi(v); perr == nil && h > 0 {
			raw = time.Duration(h) * time.Hour
		}
	}
	if v, ok, gerr := r.Get(ctx, KeyMetricsHourlyRetentionHours); gerr != nil {
		return 0, 0, gerr
	} else if ok {
		if h, perr := strconv.Atoi(v); perr == nil && h > 0 {
			hourly = time.Duration(h) * time.Hour
		}
	}
	return raw, hourly, nil
}

// SetMetricsRetention persists an override for both windows. Both must be
// positive — clearing back to "use the default" isn't exposed via the API
// since the default is itself always a sane, documented value.
func (r *Repo) SetMetricsRetention(ctx context.Context, raw, hourly time.Duration) error {
	if raw <= 0 || hourly <= 0 {
		return fmt.Errorf("appsettings: retention values must be positive")
	}
	if err := r.Set(ctx, KeyMetricsRawRetentionHours, strconv.Itoa(int(raw.Hours()))); err != nil {
		return err
	}
	return r.Set(ctx, KeyMetricsHourlyRetentionHours, strconv.Itoa(int(hourly.Hours())))
}
