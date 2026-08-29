-- Network traffic history, uploaded in batches from the agent's local
-- buffer (internal/netmetrics) rather than pushed live per-sample like
-- device_metrics — see docs/AGENT.md "Netzwerk-Traffic-Metriken". Kept as
-- its own table rather than more columns on device_metrics because the
-- sampling cadence differs (much finer-grained) and each row already
-- carries its own interval_seconds rather than assuming a fixed one.
--
-- *_total columns are system-wide (all network interfaces on the
-- device); *_control columns are just this agent's own control-channel
-- traffic to this server, tracked separately so "how much bandwidth does
-- this machine use in general" and "how much overhead does this tool
-- itself add" can be told apart.
CREATE TABLE device_network_metrics (
    id                  bigserial PRIMARY KEY,
    device_id           uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    observed_at         timestamptz NOT NULL,
    interval_seconds    real NOT NULL,
    bytes_sent_total    bigint NOT NULL,
    bytes_recv_total    bigint NOT NULL,
    bytes_sent_control  bigint NOT NULL,
    bytes_recv_control  bigint NOT NULL
);

CREATE INDEX idx_device_network_metrics_device_time ON device_network_metrics(device_id, observed_at DESC);

-- Hourly rollups store SUMs (total bytes transferred that hour), not
-- averages like device_metrics_hourly's avg_cpu_percent etc. — "total
-- bytes this hour" is the meaningful hourly figure for a volume metric,
-- not "average bytes per sample". sum_interval_seconds lets the API
-- recompute an accurate average throughput (sum_bytes / sum_interval)
-- without assuming every sample used the nominal interval.
CREATE TABLE device_network_metrics_hourly (
    device_id               uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    bucket_start            timestamptz NOT NULL,
    sum_interval_seconds    real NOT NULL,
    sum_bytes_sent_total    bigint NOT NULL,
    sum_bytes_recv_total    bigint NOT NULL,
    sum_bytes_sent_control  bigint NOT NULL,
    sum_bytes_recv_control  bigint NOT NULL,
    sample_count            integer NOT NULL,
    PRIMARY KEY (device_id, bucket_start)
);

CREATE INDEX idx_device_network_metrics_hourly_device_time ON device_network_metrics_hourly(device_id, bucket_start DESC);
