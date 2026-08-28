-- Disk usage history (summed across non-removable filesystems per sample —
-- the raw per-filesystem breakdown already lives in device_metrics.filesystems,
-- this is just the aggregate needed for a single trend line/hourly rollup,
-- same pattern as cpu_percent/memory_used_bytes).
ALTER TABLE device_metrics ADD COLUMN disk_used_bytes bigint;
ALTER TABLE device_metrics ADD COLUMN disk_total_bytes bigint;

ALTER TABLE device_metrics_hourly ADD COLUMN avg_disk_used_bytes bigint;
ALTER TABLE device_metrics_hourly ADD COLUMN avg_disk_total_bytes bigint;

-- Runtime-adjustable settings (e.g. metrics retention), overriding the
-- static server.yaml defaults without a restart. Absence of a key means
-- "use the configured default" — see internal/appsettings.
CREATE TABLE app_settings (
    key         text PRIMARY KEY,
    value       text NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);
