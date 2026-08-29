// Package device implements the device registry: reading, listing and
// updating enrolled devices, and recording inventory/metrics. See
// docs/DATABASE.md and docs/API.md §5-6.
package device

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"wartungsremote/internal/protocol"
)

var ErrNotFound = errors.New("device: not found")

type Status string

const (
	StatusUnknown        Status = "unknown"
	StatusOnline         Status = "online"
	StatusConnectionLost Status = "connection_lost"
	StatusOffline        Status = "offline"
	StatusRevoked        Status = "revoked"
)

type Health string

const (
	HealthHealthy  Health = "healthy"
	HealthWarning  Health = "warning"
	HealthCritical Health = "critical"
	HealthOffline  Health = "offline"
	HealthUnknown  Health = "unknown"
)

type Device struct {
	ID               uuid.UUID
	InstallID        uuid.UUID
	CustomerID       *uuid.UUID
	GroupID          *uuid.UUID
	DisplayName      string
	Hostname         string
	OSFamily         string
	OSName           string
	OSVersion        string
	Architecture     string
	AgentVersion     string
	Status           Status
	Health           Health
	HealthReasons    []string
	Tags             []string
	LastSeenAt       *time.Time
	LastPublicIP     string
	CredentialStatus string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo {
	return &Repo{pool: pool}
}

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	var healthReasons, tags []byte
	var lastPublicIP *string
	err := row.Scan(
		&d.ID, &d.InstallID, &d.CustomerID, &d.GroupID, &d.DisplayName, &d.Hostname,
		&d.OSFamily, &d.OSName, &d.OSVersion, &d.Architecture, &d.AgentVersion,
		&d.Status, &d.Health, &healthReasons, &tags, &d.LastSeenAt, &lastPublicIP,
		&d.CredentialStatus, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return Device{}, err
	}
	_ = json.Unmarshal(healthReasons, &d.HealthReasons)
	_ = json.Unmarshal(tags, &d.Tags)
	if lastPublicIP != nil {
		d.LastPublicIP = *lastPublicIP
	}
	return d, nil
}

const selectColumns = `
	id, install_id, customer_id, group_id, display_name, COALESCE(hostname,''),
	COALESCE(os_family,''), COALESCE(os_name,''), COALESCE(os_version,''), COALESCE(architecture,''), COALESCE(agent_version,''),
	status, health, health_reasons, tags, last_seen_at, host(last_public_ip),
	credential_status, created_at, updated_at
`

func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (Device, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+selectColumns+` FROM devices WHERE id = $1`, id)
	d, err := scanDevice(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("device: get by id: %w", err)
	}
	return d, nil
}

func (r *Repo) GetByInstallID(ctx context.Context, installID uuid.UUID) (Device, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+selectColumns+` FROM devices WHERE install_id = $1`, installID)
	d, err := scanDevice(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	if err != nil {
		return Device{}, fmt.Errorf("device: get by install id: %w", err)
	}
	return d, nil
}

type ListFilter struct {
	Status     string
	Health     string
	CustomerID *uuid.UUID
	GroupID    *uuid.UUID
	Tag        string
	Query      string
	Page       int
	PageSize   int
}

func (r *Repo) List(ctx context.Context, f ListFilter) ([]Device, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 200 {
		f.PageSize = 50
	}

	where := "WHERE 1=1"
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if f.Status != "" {
		where += " AND status = " + arg(f.Status)
	}
	if f.Health != "" {
		where += " AND health = " + arg(f.Health)
	}
	if f.CustomerID != nil {
		where += " AND customer_id = " + arg(*f.CustomerID)
	}
	if f.GroupID != nil {
		where += " AND group_id = " + arg(*f.GroupID)
	}
	if f.Tag != "" {
		where += " AND tags ? " + arg(f.Tag)
	}
	if f.Query != "" {
		where += " AND (display_name ILIKE " + arg("%"+f.Query+"%") + " OR hostname ILIKE " + arg("%"+f.Query+"%") + ")"
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM devices `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("device: count: %w", err)
	}

	limitArg := arg(f.PageSize)
	offsetArg := arg((f.Page - 1) * f.PageSize)
	rows, err := r.pool.Query(ctx, `SELECT `+selectColumns+` FROM devices `+where+` ORDER BY display_name LIMIT `+limitArg+` OFFSET `+offsetArg, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("device: list: %w", err)
	}
	defer rows.Close()

	var out []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("device: scan list row: %w", err)
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

// ListAll returns every non-revoked device, unpaginated. Used by the
// alerting engine to evaluate global-scoped rules.
func (r *Repo) ListAll(ctx context.Context) ([]Device, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+selectColumns+` FROM devices WHERE status != 'revoked' ORDER BY display_name`)
	if err != nil {
		return nil, fmt.Errorf("device: list all: %w", err)
	}
	defer rows.Close()

	var out []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("device: scan list-all row: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PatchInput carries the subset of device fields an admin may change
// directly, per docs/API.md §5 PATCH /devices/:id. Cryptographic identity is
// never patchable through this path.
type PatchInput struct {
	DisplayName *string
	CustomerID  **uuid.UUID
	GroupID     **uuid.UUID
	Tags        *[]string
	Policy      map[string]any
}

func (r *Repo) Patch(ctx context.Context, id uuid.UUID, in PatchInput) error {
	sets := []string{"updated_at = now()"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if in.DisplayName != nil {
		sets = append(sets, "display_name = "+arg(*in.DisplayName))
	}
	if in.CustomerID != nil {
		sets = append(sets, "customer_id = "+arg(*in.CustomerID))
	}
	if in.GroupID != nil {
		sets = append(sets, "group_id = "+arg(*in.GroupID))
	}
	if in.Tags != nil {
		tagsJSON, _ := json.Marshal(*in.Tags)
		sets = append(sets, "tags = "+arg(tagsJSON)+"::jsonb")
	}
	if in.Policy != nil {
		policyJSON, _ := json.Marshal(in.Policy)
		sets = append(sets, "policy = "+arg(policyJSON)+"::jsonb")
	}
	idArg := arg(id)

	query := "UPDATE devices SET " + joinComma(sets) + " WHERE id = " + idArg
	tag, err := r.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("device: patch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// UpdateConnectivity sets status/last_seen_at/last_public_ip on
// (re)connect, heartbeat, or disconnect transitions. See
// docs/STATE_MACHINES.md §1.
func (r *Repo) UpdateConnectivity(ctx context.Context, id uuid.UUID, status Status, publicIP string) error {
	var ipVal any
	if publicIP != "" {
		ipVal = publicIP
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE devices SET status = $2, last_seen_at = now(), last_public_ip = COALESCE($3, last_public_ip), updated_at = now()
		WHERE id = $1
	`, id, string(status), ipVal)
	if err != nil {
		return fmt.Errorf("device: update connectivity: %w", err)
	}
	return nil
}

func (r *Repo) UpdateStatusOnly(ctx context.Context, id uuid.UUID, status Status) error {
	_, err := r.pool.Exec(ctx, `UPDATE devices SET status = $2, updated_at = now() WHERE id = $1`, id, string(status))
	if err != nil {
		return fmt.Errorf("device: update status: %w", err)
	}
	return nil
}

// UpdateHealth stores the outcome of the health evaluation engine.
func (r *Repo) UpdateHealth(ctx context.Context, id uuid.UUID, health Health, reasons []string) error {
	reasonsJSON, _ := json.Marshal(reasons)
	_, err := r.pool.Exec(ctx, `UPDATE devices SET health = $2, health_reasons = $3, updated_at = now() WHERE id = $1`, id, string(health), reasonsJSON)
	if err != nil {
		return fmt.Errorf("device: update health: %w", err)
	}
	return nil
}

// ApplyInventory stores a full inventory snapshot's summary fields onto the
// device row (docs/PROTOCOL.md §6). Fine-grained history is not kept for
// inventory itself; only current state.
func (r *Repo) ApplyInventory(ctx context.Context, id uuid.UUID, hostname, osFamily, osName, osVersion, arch, agentVersion string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE devices SET hostname=$2, os_family=$3, os_name=$4, os_version=$5, architecture=$6, agent_version=$7, updated_at=now()
		WHERE id = $1
	`, id, hostname, osFamily, osName, osVersion, arch, agentVersion)
	if err != nil {
		return fmt.Errorf("device: apply inventory: %w", err)
	}
	return nil
}

func (r *Repo) RecordNetwork(ctx context.Context, id uuid.UUID, interfaces any, publicIP string) error {
	interfacesJSON, err := json.Marshal(interfaces)
	if err != nil {
		return fmt.Errorf("device: marshal interfaces: %w", err)
	}
	var ipVal any
	if publicIP != "" {
		ipVal = publicIP
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO device_network (device_id, interfaces, public_ip) VALUES ($1,$2,$3)`, id, interfacesJSON, ipVal)
	if err != nil {
		return fmt.Errorf("device: record network: %w", err)
	}
	return nil
}

// IPHistoryEntry is one distinct public IP observed for a device within a
// lookback window, with when it was first/last seen in that window.
type IPHistoryEntry struct {
	IP        string
	FirstSeen time.Time
	LastSeen  time.Time
}

// RecentIPHistory returns every distinct public IP recorded for a device
// within the last `since` duration, newest-observed first — the data
// behind "how many different IPs has this device had recently" (e.g. a
// hover tooltip next to the current IP on the device overview).
func (r *Repo) RecentIPHistory(ctx context.Context, id uuid.UUID, since time.Duration) ([]IPHistoryEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT host(public_ip), min(observed_at), max(observed_at)
		FROM device_network
		WHERE device_id = $1 AND public_ip IS NOT NULL AND observed_at >= now() - make_interval(secs => $2)
		GROUP BY public_ip
		ORDER BY max(observed_at) DESC
	`, id, since.Seconds())
	if err != nil {
		return nil, fmt.Errorf("device: recent ip history: %w", err)
	}
	defer rows.Close()

	out := []IPHistoryEntry{}
	for rows.Next() {
		var e IPHistoryEntry
		if err := rows.Scan(&e.IP, &e.FirstSeen, &e.LastSeen); err != nil {
			return nil, fmt.Errorf("device: scan ip history: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repo) RecordMetrics(ctx context.Context, id uuid.UUID, cpuPercent float64, memUsed, memTotal uint64, filesystems []protocol.FilesystemUsage, uptimeSeconds int64) error {
	fsJSON, err := json.Marshal(filesystems)
	if err != nil {
		return fmt.Errorf("device: marshal filesystems: %w", err)
	}
	// Aggregated across non-removable filesystems only — a full USB stick
	// shouldn't move the "disk usage" trend line any more than it should
	// trip a health alert (see FilesystemUsage.Removable).
	var diskUsed, diskTotal uint64
	for _, fs := range filesystems {
		if fs.Removable {
			continue
		}
		diskUsed += fs.UsedBytes
		diskTotal += fs.TotalBytes
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO device_metrics (device_id, cpu_percent, memory_used_bytes, memory_total_bytes, filesystems, uptime_seconds, disk_used_bytes, disk_total_bytes)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, id, cpuPercent, memUsed, memTotal, fsJSON, uptimeSeconds, int64(diskUsed), int64(diskTotal))
	if err != nil {
		return fmt.Errorf("device: record metrics: %w", err)
	}
	return nil
}

// NetworkMetricsPoint is one row of network traffic history, raw or
// hourly-rolled-up (BucketStart == ObservedAt for raw rows).
type NetworkMetricsPoint struct {
	ObservedAt       time.Time
	IntervalSeconds  float64
	BytesSentTotal   int64
	BytesRecvTotal   int64
	BytesSentControl int64
	BytesRecvControl int64
}

// RecordNetworkMetricsBatch bulk-inserts a batch of locally-buffered
// agent samples (protocol.NetworkMetricsBatchPayload) in one round trip.
func (r *Repo) RecordNetworkMetricsBatch(ctx context.Context, id uuid.UUID, samples []protocol.NetworkMetricsSample) error {
	if len(samples) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, s := range samples {
		batch.Queue(`
			INSERT INTO device_network_metrics
				(device_id, observed_at, interval_seconds, bytes_sent_total, bytes_recv_total, bytes_sent_control, bytes_recv_control)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, id, s.OccurredAt, s.IntervalSeconds, int64(s.BytesSentTotal), int64(s.BytesRecvTotal), int64(s.BytesSentControl), int64(s.BytesRecvControl))
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range samples {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("device: record network metrics batch: %w", err)
		}
	}
	return nil
}

// RecentNetworkMetrics returns raw network samples in a time range.
func (r *Repo) RecentNetworkMetrics(ctx context.Context, id uuid.UUID, from, to time.Time, limit int) ([]NetworkMetricsPoint, error) {
	if limit <= 0 || limit > 5000 {
		limit = 2000
	}
	rows, err := r.pool.Query(ctx, `
		SELECT observed_at, interval_seconds, bytes_sent_total, bytes_recv_total, bytes_sent_control, bytes_recv_control
		FROM device_network_metrics
		WHERE device_id = $1 AND observed_at BETWEEN $2 AND $3
		ORDER BY observed_at DESC
		LIMIT $4
	`, id, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("device: recent network metrics: %w", err)
	}
	defer rows.Close()

	var out []NetworkMetricsPoint
	for rows.Next() {
		var p NetworkMetricsPoint
		if err := rows.Scan(&p.ObservedAt, &p.IntervalSeconds, &p.BytesSentTotal, &p.BytesRecvTotal, &p.BytesSentControl, &p.BytesRecvControl); err != nil {
			return nil, fmt.Errorf("device: scan network metrics point: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// HourlyNetworkMetrics returns hourly-summed network traffic points —
// IntervalSeconds/byte fields here are the hour's totals (see
// migrations/0010_network_metrics.sql), used to compute an accurate
// average throughput for the whole hour.
func (r *Repo) HourlyNetworkMetrics(ctx context.Context, id uuid.UUID, from, to time.Time, limit int) ([]NetworkMetricsPoint, error) {
	if limit <= 0 || limit > 5000 {
		limit = 2000
	}
	rows, err := r.pool.Query(ctx, `
		SELECT bucket_start, sum_interval_seconds, sum_bytes_sent_total, sum_bytes_recv_total, sum_bytes_sent_control, sum_bytes_recv_control
		FROM device_network_metrics_hourly
		WHERE device_id = $1 AND bucket_start BETWEEN $2 AND $3
		ORDER BY bucket_start DESC
		LIMIT $4
	`, id, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("device: hourly network metrics: %w", err)
	}
	defer rows.Close()

	var out []NetworkMetricsPoint
	for rows.Next() {
		var p NetworkMetricsPoint
		if err := rows.Scan(&p.ObservedAt, &p.IntervalSeconds, &p.BytesSentTotal, &p.BytesRecvTotal, &p.BytesSentControl, &p.BytesRecvControl); err != nil {
			return nil, fmt.Errorf("device: scan hourly network metrics point: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RollupHourlyNetworkMetrics upserts hourly SUM aggregates for every raw
// device_network_metrics row older than the current hour boundary —
// SUM rather than avg (unlike RollupHourlyMetrics) because "total bytes
// this hour" is the meaningful figure for a volume metric.
func (r *Repo) RollupHourlyNetworkMetrics(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO device_network_metrics_hourly
			(device_id, bucket_start, sum_interval_seconds, sum_bytes_sent_total, sum_bytes_recv_total, sum_bytes_sent_control, sum_bytes_recv_control, sample_count)
		SELECT device_id, date_trunc('hour', observed_at) AS bucket_start,
		       sum(interval_seconds), sum(bytes_sent_total), sum(bytes_recv_total), sum(bytes_sent_control), sum(bytes_recv_control), count(*)
		FROM device_network_metrics
		WHERE observed_at < date_trunc('hour', now())
		GROUP BY device_id, date_trunc('hour', observed_at)
		ON CONFLICT (device_id, bucket_start) DO UPDATE SET
			sum_interval_seconds = EXCLUDED.sum_interval_seconds,
			sum_bytes_sent_total = EXCLUDED.sum_bytes_sent_total,
			sum_bytes_recv_total = EXCLUDED.sum_bytes_recv_total,
			sum_bytes_sent_control = EXCLUDED.sum_bytes_sent_control,
			sum_bytes_recv_control = EXCLUDED.sum_bytes_recv_control,
			sample_count = EXCLUDED.sample_count
	`)
	if err != nil {
		return fmt.Errorf("device: rollup hourly network metrics: %w", err)
	}
	return nil
}

// ApplyNetworkMetricsRetention mirrors ApplyMetricsRetention for the
// separate network metrics tables.
func (r *Repo) ApplyNetworkMetricsRetention(ctx context.Context, rawRetention, hourlyRetention time.Duration) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM device_network_metrics WHERE observed_at < now() - make_interval(secs => $1)`, rawRetention.Seconds()); err != nil {
		return fmt.Errorf("device: raw network metrics retention: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM device_network_metrics_hourly WHERE bucket_start < now() - make_interval(secs => $1)`, hourlyRetention.Seconds()); err != nil {
		return fmt.Errorf("device: hourly network metrics retention: %w", err)
	}
	return nil
}

// DeviceNetworkTotal is one row of the cross-device traffic ranking —
// "which client is using how much bandwidth" (docs/API.md §6).
type DeviceNetworkTotal struct {
	DeviceID         uuid.UUID
	DisplayName      string
	BytesSentTotal   int64
	BytesRecvTotal   int64
	BytesSentControl int64
	BytesRecvControl int64
}

// NetworkUsageSummary sums raw network samples since `since`, per device,
// for the cross-device ranking view — deliberately queries the raw table
// only (not hourly), so `since` should stay within the raw retention
// window (see appsettings.KeyNetworkRawRetentionHours) or totals will be
// incomplete for older devices' history.
func (r *Repo) NetworkUsageSummary(ctx context.Context, since time.Time) ([]DeviceNetworkTotal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT d.id, d.display_name,
		       COALESCE(sum(m.bytes_sent_total),0), COALESCE(sum(m.bytes_recv_total),0),
		       COALESCE(sum(m.bytes_sent_control),0), COALESCE(sum(m.bytes_recv_control),0)
		FROM devices d
		JOIN device_network_metrics m ON m.device_id = d.id AND m.observed_at >= $1
		GROUP BY d.id, d.display_name
		ORDER BY sum(m.bytes_sent_total) + sum(m.bytes_recv_total) DESC
	`, since)
	if err != nil {
		return nil, fmt.Errorf("device: network usage summary: %w", err)
	}
	defer rows.Close()

	var out []DeviceNetworkTotal
	for rows.Next() {
		var t DeviceNetworkTotal
		if err := rows.Scan(&t.DeviceID, &t.DisplayName, &t.BytesSentTotal, &t.BytesRecvTotal, &t.BytesSentControl, &t.BytesRecvControl); err != nil {
			return nil, fmt.Errorf("device: scan network usage summary: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ActiveCredentialPublicKey returns the current (highest key_version,
// non-revoked, non-expired) Ed25519 public key for the device's control
// channel authentication.
func (r *Repo) ActiveCredentialPublicKey(ctx context.Context, id uuid.UUID) ([]byte, error) {
	var pub []byte
	err := r.pool.QueryRow(ctx, `
		SELECT public_key FROM device_credentials
		WHERE device_id = $1 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > now())
		ORDER BY key_version DESC LIMIT 1
	`, id).Scan(&pub)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("device: active credential: %w", err)
	}
	return pub, nil
}

// Revoke marks the device and all its credentials as revoked, per
// docs/API.md §5 POST /devices/:id/revoke and docs/SECURITY.md §20.
func (r *Repo) Revoke(ctx context.Context, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("device: begin revoke tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `UPDATE devices SET status='revoked', credential_status='revoked', revoked_at=now(), updated_at=now() WHERE id=$1`, id); err != nil {
		return fmt.Errorf("device: revoke device: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE device_credentials SET revoked_at=now() WHERE device_id=$1 AND revoked_at IS NULL`, id); err != nil {
		return fmt.Errorf("device: revoke credentials: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *Repo) SetCapabilities(ctx context.Context, id uuid.UUID, capabilities []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("device: begin capabilities tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM device_capabilities WHERE device_id = $1`, id); err != nil {
		return fmt.Errorf("device: clear capabilities: %w", err)
	}
	for _, cap := range capabilities {
		if _, err := tx.Exec(ctx, `INSERT INTO device_capabilities (device_id, capability) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, cap); err != nil {
			return fmt.Errorf("device: insert capability: %w", err)
		}
	}
	return tx.Commit(ctx)
}

type MetricsPoint struct {
	ObservedAt       time.Time
	CPUPercent       float64
	MemoryUsedBytes  int64
	MemoryTotalBytes int64
	UptimeSeconds    int64
	DiskUsedBytes    int64
	DiskTotalBytes   int64
	Filesystems      []protocol.FilesystemUsage
}

// LatestMetrics returns the single most recent metrics row, including
// per-filesystem usage, for health evaluation and the device detail view.
func (r *Repo) LatestMetrics(ctx context.Context, id uuid.UUID) (MetricsPoint, bool, error) {
	var p MetricsPoint
	var fsJSON []byte
	err := r.pool.QueryRow(ctx, `
		SELECT observed_at, COALESCE(cpu_percent,0), COALESCE(memory_used_bytes,0), COALESCE(memory_total_bytes,0), COALESCE(uptime_seconds,0), filesystems
		FROM device_metrics WHERE device_id = $1 ORDER BY observed_at DESC LIMIT 1
	`, id).Scan(&p.ObservedAt, &p.CPUPercent, &p.MemoryUsedBytes, &p.MemoryTotalBytes, &p.UptimeSeconds, &fsJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return MetricsPoint{}, false, nil
	}
	if err != nil {
		return MetricsPoint{}, false, fmt.Errorf("device: latest metrics: %w", err)
	}
	_ = json.Unmarshal(fsJSON, &p.Filesystems)
	return p, true, nil
}

func (r *Repo) RecentMetrics(ctx context.Context, id uuid.UUID, from, to time.Time, limit int) ([]MetricsPoint, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := r.pool.Query(ctx, `
		SELECT observed_at, COALESCE(cpu_percent,0), COALESCE(memory_used_bytes,0), COALESCE(memory_total_bytes,0), COALESCE(uptime_seconds,0), COALESCE(disk_used_bytes,0), COALESCE(disk_total_bytes,0)
		FROM device_metrics
		WHERE device_id = $1 AND observed_at BETWEEN $2 AND $3
		ORDER BY observed_at DESC
		LIMIT $4
	`, id, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("device: recent metrics: %w", err)
	}
	defer rows.Close()

	var out []MetricsPoint
	for rows.Next() {
		var p MetricsPoint
		if err := rows.Scan(&p.ObservedAt, &p.CPUPercent, &p.MemoryUsedBytes, &p.MemoryTotalBytes, &p.UptimeSeconds, &p.DiskUsedBytes, &p.DiskTotalBytes); err != nil {
			return nil, fmt.Errorf("device: scan metrics point: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// HourlyMetrics returns downsampled hourly-average metrics points, used for
// chart ranges too wide to render usefully from raw 5-minute samples.
func (r *Repo) HourlyMetrics(ctx context.Context, id uuid.UUID, from, to time.Time, limit int) ([]MetricsPoint, error) {
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := r.pool.Query(ctx, `
		SELECT bucket_start, COALESCE(avg_cpu_percent,0), COALESCE(avg_memory_used_bytes,0), COALESCE(avg_memory_total_bytes,0), COALESCE(avg_disk_used_bytes,0), COALESCE(avg_disk_total_bytes,0)
		FROM device_metrics_hourly
		WHERE device_id = $1 AND bucket_start BETWEEN $2 AND $3
		ORDER BY bucket_start DESC
		LIMIT $4
	`, id, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("device: hourly metrics: %w", err)
	}
	defer rows.Close()

	var out []MetricsPoint
	for rows.Next() {
		var p MetricsPoint
		if err := rows.Scan(&p.ObservedAt, &p.CPUPercent, &p.MemoryUsedBytes, &p.MemoryTotalBytes, &p.DiskUsedBytes, &p.DiskTotalBytes); err != nil {
			return nil, fmt.Errorf("device: scan hourly metrics point: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// RollupHourlyMetrics upserts hourly-average aggregates for every raw
// device_metrics row older than the current hour boundary. Idempotent by
// design (ON CONFLICT ... DO UPDATE) so it can simply be re-run on every
// sweep tick rather than tracking which hours were already rolled up.
func (r *Repo) RollupHourlyMetrics(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO device_metrics_hourly (device_id, bucket_start, avg_cpu_percent, avg_memory_used_bytes, avg_memory_total_bytes, avg_disk_used_bytes, avg_disk_total_bytes, sample_count)
		SELECT device_id, date_trunc('hour', observed_at) AS bucket_start,
		       avg(cpu_percent), avg(memory_used_bytes)::bigint, avg(memory_total_bytes)::bigint, avg(disk_used_bytes)::bigint, avg(disk_total_bytes)::bigint, count(*)
		FROM device_metrics
		WHERE observed_at < date_trunc('hour', now())
		GROUP BY device_id, date_trunc('hour', observed_at)
		ON CONFLICT (device_id, bucket_start) DO UPDATE SET
			avg_cpu_percent = EXCLUDED.avg_cpu_percent,
			avg_memory_used_bytes = EXCLUDED.avg_memory_used_bytes,
			avg_memory_total_bytes = EXCLUDED.avg_memory_total_bytes,
			avg_disk_used_bytes = EXCLUDED.avg_disk_used_bytes,
			avg_disk_total_bytes = EXCLUDED.avg_disk_total_bytes,
			sample_count = EXCLUDED.sample_count
	`)
	if err != nil {
		return fmt.Errorf("device: rollup hourly metrics: %w", err)
	}
	return nil
}

// ApplyMetricsRetention deletes raw and hourly metrics rows older than the
// configured retention windows, per docs/DATABASE.md §3.
func (r *Repo) ApplyMetricsRetention(ctx context.Context, rawRetention, hourlyRetention time.Duration) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM device_metrics WHERE observed_at < now() - make_interval(secs => $1)`, rawRetention.Seconds()); err != nil {
		return fmt.Errorf("device: raw metrics retention: %w", err)
	}
	if _, err := r.pool.Exec(ctx, `DELETE FROM device_metrics_hourly WHERE bucket_start < now() - make_interval(secs => $1)`, hourlyRetention.Seconds()); err != nil {
		return fmt.Errorf("device: hourly metrics retention: %w", err)
	}
	return nil
}
