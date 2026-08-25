package device

import (
	"context"
	"log/slog"
	"time"
)

// RunMetricsRetentionSweeper periodically rolls up raw metrics into hourly
// aggregates and prunes rows past their retention window (docs/DATABASE.md
// §3, docs/CONFIGURATION.md §1 metrics.raw_retention/hourly_retention).
func RunMetricsRetentionSweeper(ctx context.Context, repo *Repo, rawRetention, hourlyRetention, interval time.Duration) {
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
			if err := repo.ApplyMetricsRetention(ctx, rawRetention, hourlyRetention); err != nil {
				slog.Error("metrics retention failed", "error", err)
			}
		}
	}
}
