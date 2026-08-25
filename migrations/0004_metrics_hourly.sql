-- Hourly downsampled metrics aggregates, per docs/DATABASE.md §3 retention
-- policy (raw 5-minute samples kept ~30 days, hourly aggregates ~365 days).
CREATE TABLE device_metrics_hourly (
    device_id           uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    bucket_start         timestamptz NOT NULL,
    avg_cpu_percent      real,
    avg_memory_used_bytes  bigint,
    avg_memory_total_bytes bigint,
    sample_count         integer NOT NULL,
    PRIMARY KEY (device_id, bucket_start)
);

CREATE INDEX idx_device_metrics_hourly_device_time ON device_metrics_hourly(device_id, bucket_start DESC);
