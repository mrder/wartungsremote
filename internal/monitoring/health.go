// Package monitoring implements the device health evaluation engine per
// docs/SPECIFICATION.md §11 and docs/PROJECT_CONCEPT.md §11.
package monitoring

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"wartungsremote/internal/device"
)

// Thresholds are the V1 default rules; server-configurable per spec (kept as
// a struct with defaults for now; a future settings API can override them).
type Thresholds struct {
	DiskWarningPercent    float64
	DiskCriticalPercent   float64
	RAMWarningPercent     float64
	CPUWarningPercent     float64
	SustainedWindow       time.Duration
	MinSupportedAgentVer  string
}

func DefaultThresholds() Thresholds {
	return Thresholds{
		DiskWarningPercent:   90,
		DiskCriticalPercent:  97,
		RAMWarningPercent:    90,
		CPUWarningPercent:    95,
		SustainedWindow:      10 * time.Minute,
		MinSupportedAgentVer: "0.1.0",
	}
}

type Engine struct {
	devices    *device.Repo
	thresholds Thresholds
}

func NewEngine(devices *device.Repo, thresholds Thresholds) *Engine {
	return &Engine{devices: devices, thresholds: thresholds}
}

// Evaluate computes health for a single device and persists the result. It
// is called after every metrics_report and on status-request, and should
// also be scheduled periodically so purely time-based conditions (sustained
// CPU/RAM, offline) get re-evaluated even without new agent traffic.
func (e *Engine) Evaluate(ctx context.Context, deviceID uuid.UUID) (device.Health, []string, error) {
	d, err := e.devices.GetByID(ctx, deviceID)
	if err != nil {
		return "", nil, fmt.Errorf("monitoring: load device: %w", err)
	}

	if d.Status == device.StatusOffline || d.Status == device.StatusRevoked {
		health := device.HealthOffline
		if d.Status == device.StatusRevoked {
			health = device.HealthUnknown
		}
		if err := e.devices.UpdateHealth(ctx, deviceID, health, nil); err != nil {
			return "", nil, err
		}
		return health, nil, nil
	}

	var reasons []string
	critical := false
	warning := false

	latest, ok, err := e.devices.LatestMetrics(ctx, deviceID)
	if err != nil {
		return "", nil, fmt.Errorf("monitoring: latest metrics: %w", err)
	}
	if ok {
		for _, fs := range latest.Filesystems {
			if fs.TotalBytes == 0 || fs.Removable {
				continue
			}
			pct := float64(fs.UsedBytes) / float64(fs.TotalBytes) * 100
			switch {
			case pct >= e.thresholds.DiskCriticalPercent:
				critical = true
				reasons = append(reasons, fmt.Sprintf("disk %s at %.1f%% (critical >= %.0f%%)", fs.Path, pct, e.thresholds.DiskCriticalPercent))
			case pct >= e.thresholds.DiskWarningPercent:
				warning = true
				reasons = append(reasons, fmt.Sprintf("disk %s at %.1f%% (warning >= %.0f%%)", fs.Path, pct, e.thresholds.DiskWarningPercent))
			}
		}

		sustainedFrom := time.Now().UTC().Add(-e.thresholds.SustainedWindow)
		history, err := e.devices.RecentMetrics(ctx, deviceID, sustainedFrom, time.Now().UTC(), 200)
		if err != nil {
			return "", nil, fmt.Errorf("monitoring: recent metrics: %w", err)
		}
		if sustainedAbove(history, e.thresholds.SustainedWindow, func(p device.MetricsPoint) bool {
			return p.CPUPercent >= e.thresholds.CPUWarningPercent
		}) {
			warning = true
			reasons = append(reasons, fmt.Sprintf("cpu >= %.0f%% for >= %s", e.thresholds.CPUWarningPercent, e.thresholds.SustainedWindow))
		}
		if sustainedAbove(history, e.thresholds.SustainedWindow, func(p device.MetricsPoint) bool {
			if p.MemoryTotalBytes == 0 {
				return false
			}
			return float64(p.MemoryUsedBytes)/float64(p.MemoryTotalBytes)*100 >= e.thresholds.RAMWarningPercent
		}) {
			warning = true
			reasons = append(reasons, fmt.Sprintf("ram >= %.0f%% for >= %s", e.thresholds.RAMWarningPercent, e.thresholds.SustainedWindow))
		}
	}

	if d.AgentVersion != "" && IsOlderVersion(d.AgentVersion, e.thresholds.MinSupportedAgentVer) {
		warning = true
		reasons = append(reasons, fmt.Sprintf("agent version %s is older than minimum supported %s", d.AgentVersion, e.thresholds.MinSupportedAgentVer))
	}

	health := device.HealthHealthy
	switch {
	case critical:
		health = device.HealthCritical
	case warning:
		health = device.HealthWarning
	}

	if err := e.devices.UpdateHealth(ctx, deviceID, health, reasons); err != nil {
		return "", nil, err
	}
	return health, reasons, nil
}

// sustainedAbove reports whether every sample within the window satisfies
// pred and at least one sample exists at (or before) window start, i.e. we
// have continuous coverage of the whole window above threshold. This is a
// deliberately conservative approximation: a single below-threshold sample
// anywhere in the window clears the condition.
func sustainedAbove(history []device.MetricsPoint, window time.Duration, pred func(device.MetricsPoint) bool) bool {
	if len(history) == 0 {
		return false
	}
	oldest := history[0].ObservedAt
	for _, p := range history {
		if !pred(p) {
			return false
		}
		if p.ObservedAt.Before(oldest) {
			oldest = p.ObservedAt
		}
	}
	newest := history[0].ObservedAt
	for _, p := range history {
		if p.ObservedAt.After(newest) {
			newest = p.ObservedAt
		}
	}
	return newest.Sub(oldest) >= window-time.Minute // small tolerance for jittered report intervals
}

// IsOlderVersion does a best-effort dotted-numeric comparison; malformed
// versions are treated as not-older (fail open on the version check only,
// never on security-relevant checks).
func IsOlderVersion(version, minimum string) bool {
	vp, err1 := ParseVersion(version)
	mp, err2 := ParseVersion(minimum)
	if err1 != nil || err2 != nil {
		return false
	}
	for i := 0; i < 3; i++ {
		if vp[i] != mp[i] {
			return vp[i] < mp[i]
		}
	}
	return false
}

func ParseVersion(v string) ([3]int, error) {
	var out [3]int
	var part, idx int
	for i := 0; i <= len(v); i++ {
		if i == len(v) || v[i] == '.' {
			if idx > 2 {
				return out, fmt.Errorf("monitoring: version has too many components: %s", v)
			}
			n, err := atoiStrict(v[part:i])
			if err != nil {
				return out, err
			}
			out[idx] = n
			idx++
			part = i + 1
		}
	}
	return out, nil
}

func atoiStrict(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("monitoring: empty version component")
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("monitoring: invalid version component %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
