package device

import (
	"context"
	"log/slog"
	"time"
)

// RetentionSource supplies the current effective retention windows on
// every sweep tick (rather than a fixed value captured once at startup),
// so a runtime override (internal/appsettings, dashboard Settings page)
// takes effect without a server restart.
type RetentionSource interface {
	MetricsRetention(ctx context.Context, defaultRaw, defaultHourly time.Duration) (raw, hourly time.Duration, err error)
}

// RunMetricsRetentionSweeper periodically rolls up raw metrics into hourly
// aggregates and prunes rows past their retention window (docs/DATABASE.md
// §3, docs/CONFIGURATION.md §1 metrics.raw_retention/hourly_retention).
// defaultRaw/defaultHourly are the server.yaml values, used whenever no
// runtime override has been set.
func RunMetricsRetentionSweeper(ctx context.Context, repo *Repo, settings RetentionSource, defaultRaw, defaultHourly, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := repo.RollupHourlyMetrics(ctx); err != nil {
				slog.Error("metrics rollup failed", "error", err)
				continue
			}
			rawRetention, hourlyRetention, err := settings.MetricsRetention(ctx, defaultRaw, defaultHourly)
			if err != nil {
				slog.Error("metrics retention: failed to resolve settings, using server.yaml defaults", "error", err)
				rawRetention, hourlyRetention = defaultRaw, defaultHourly
			}
			if err := repo.ApplyMetricsRetention(ctx, rawRetention, hourlyRetention); err != nil {
				slog.Error("metrics retention failed", "error", err)
			}
		}
	}
}
