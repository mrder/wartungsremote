package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"wartungsremote/internal/audit"
	"wartungsremote/internal/controlhub"
	"wartungsremote/internal/device"
	"wartungsremote/internal/monitoring"
	"wartungsremote/internal/platform"
	"wartungsremote/internal/protocol"
)

const serviceCheckTimeout = 5 * time.Second

type Engine struct {
	rules   *Repo
	devices *device.Repo
	hub     *controlhub.Hub
	audit   *audit.Logger
}

func NewEngine(rules *Repo, devices *device.Repo, hub *controlhub.Hub, auditLogger *audit.Logger) *Engine {
	return &Engine{rules: rules, devices: devices, hub: hub, audit: auditLogger}
}

// Evaluate runs every enabled rule against every device in its scope,
// opening a new alert when a condition first triggers and auto-resolving
// it once the condition clears. Idempotent and safe to call on a fixed
// interval (see RunSweeper).
func (e *Engine) Evaluate(ctx context.Context) error {
	rules, err := e.rules.ListRules(ctx)
	if err != nil {
		return fmt.Errorf("alerting: evaluate: %w", err)
	}
	devices, err := e.devices.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("alerting: evaluate: %w", err)
	}

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, d := range scopedDevices(devices, rule) {
			triggered, severity, summary, err := e.evaluateRule(ctx, rule, d)
			if err != nil {
				slog.Error("alert rule evaluation failed", "rule_id", rule.ID, "rule_type", rule.RuleType, "device_id", d.ID, "error", err)
				continue
			}
			if triggered {
				opened, err := e.rules.OpenIfNotExists(ctx, d.ID, rule.ID, severity, summary)
				if err != nil {
					slog.Error("failed to open alert", "error", err)
					continue
				}
				if opened && e.audit != nil {
					_ = e.audit.Record(ctx, audit.Event{
						ActorType: audit.ActorSystem, DeviceID: &d.ID,
						EventType: "alert.opened", Result: audit.ResultSuccess,
						Metadata: map[string]any{"rule_type": rule.RuleType, "severity": severity, "summary": summary},
					})
				}
			} else {
				resolved, err := e.rules.AutoResolveIfOpen(ctx, d.ID, rule.ID)
				if err != nil {
					slog.Error("failed to auto-resolve alert", "error", err)
					continue
				}
				if resolved && e.audit != nil {
					_ = e.audit.Record(ctx, audit.Event{
						ActorType: audit.ActorSystem, DeviceID: &d.ID,
						EventType: "alert.resolved", Result: audit.ResultSuccess,
						Metadata: map[string]any{"rule_type": rule.RuleType, "reason": "condition_cleared"},
					})
				}
			}
		}
	}
	return nil
}

func scopedDevices(all []device.Device, rule Rule) []device.Device {
	switch rule.ScopeType {
	case ScopeGlobal:
		return all
	case ScopeCustomer:
		if rule.ScopeID == nil {
			return nil
		}
		out := []device.Device{}
		for _, d := range all {
			if d.CustomerID != nil && *d.CustomerID == *rule.ScopeID {
				out = append(out, d)
			}
		}
		return out
	case ScopeGroup:
		if rule.ScopeID == nil {
			return nil
		}
		out := []device.Device{}
		for _, d := range all {
			if d.GroupID != nil && *d.GroupID == *rule.ScopeID {
				out = append(out, d)
			}
		}
		return out
	case ScopeDevice:
		if rule.ScopeID == nil {
			return nil
		}
		for _, d := range all {
			if d.ID == *rule.ScopeID {
				return []device.Device{d}
			}
		}
		return nil
	default:
		return nil
	}
}

// evaluateRule returns whether the rule's condition currently holds for d,
// plus the severity/summary to use if it does.
func (e *Engine) evaluateRule(ctx context.Context, rule Rule, d device.Device) (triggered bool, severity, summary string, err error) {
	switch rule.RuleType {
	case RuleOffline:
		if d.Status == device.StatusOffline {
			return true, "critical", fmt.Sprintf("%s is offline", d.DisplayName), nil
		}
		return false, "", "", nil

	case RuleCPU:
		var cfg struct {
			ThresholdPercent float64 `json:"threshold_percent"`
		}
		if err := json.Unmarshal(rule.Config, &cfg); err != nil {
			return false, "", "", fmt.Errorf("invalid cpu rule config: %w", err)
		}
		latest, ok, err := e.devices.LatestMetrics(ctx, d.ID)
		if err != nil || !ok {
			return false, "", "", err
		}
		if latest.CPUPercent >= cfg.ThresholdPercent {
			return true, "warning", fmt.Sprintf("CPU at %.1f%% (threshold %.0f%%)", latest.CPUPercent, cfg.ThresholdPercent), nil
		}
		return false, "", "", nil

	case RuleRAM:
		var cfg struct {
			ThresholdPercent float64 `json:"threshold_percent"`
		}
		if err := json.Unmarshal(rule.Config, &cfg); err != nil {
			return false, "", "", fmt.Errorf("invalid ram rule config: %w", err)
		}
		latest, ok, err := e.devices.LatestMetrics(ctx, d.ID)
		if err != nil || !ok || latest.MemoryTotalBytes == 0 {
			return false, "", "", err
		}
		pct := float64(latest.MemoryUsedBytes) / float64(latest.MemoryTotalBytes) * 100
		if pct >= cfg.ThresholdPercent {
			return true, "warning", fmt.Sprintf("RAM at %.1f%% (threshold %.0f%%)", pct, cfg.ThresholdPercent), nil
		}
		return false, "", "", nil

	case RuleDisk:
		var cfg struct {
			ThresholdPercent float64 `json:"threshold_percent"`
		}
		if err := json.Unmarshal(rule.Config, &cfg); err != nil {
			return false, "", "", fmt.Errorf("invalid disk rule config: %w", err)
		}
		latest, ok, err := e.devices.LatestMetrics(ctx, d.ID)
		if err != nil || !ok {
			return false, "", "", err
		}
		for _, fs := range latest.Filesystems {
			if fs.TotalBytes == 0 {
				continue
			}
			pct := float64(fs.UsedBytes) / float64(fs.TotalBytes) * 100
			if pct >= cfg.ThresholdPercent {
				return true, "warning", fmt.Sprintf("disk %s at %.1f%% (threshold %.0f%%)", fs.Path, pct, cfg.ThresholdPercent), nil
			}
		}
		return false, "", "", nil

	case RuleAgentVersion:
		var cfg struct {
			MinimumVersion string `json:"minimum_version"`
		}
		if err := json.Unmarshal(rule.Config, &cfg); err != nil {
			return false, "", "", fmt.Errorf("invalid agent_version rule config: %w", err)
		}
		if d.AgentVersion != "" && monitoring.IsOlderVersion(d.AgentVersion, cfg.MinimumVersion) {
			return true, "warning", fmt.Sprintf("agent version %s is older than required %s", d.AgentVersion, cfg.MinimumVersion), nil
		}
		return false, "", "", nil

	case RuleService:
		var cfg struct {
			ServiceName string `json:"service_name"`
		}
		if err := json.Unmarshal(rule.Config, &cfg); err != nil {
			return false, "", "", fmt.Errorf("invalid service rule config: %w", err)
		}
		if cfg.ServiceName == "" || !e.hub.IsOnline(d.ID) {
			// Can't check a service on an offline agent; don't flap the
			// alert state based on connectivity — the offline rule covers that.
			return false, "", "", nil
		}
		env, err := e.hub.SendAndWait(ctx, d.ID, protocol.TypeDeviceCommand, protocol.DeviceCommandPayload{
			CommandType: protocol.CmdServicesList,
		}, serviceCheckTimeout)
		if err != nil {
			return false, "", "", nil // agent didn't respond in time; skip this tick
		}
		var result struct {
			Status string                  `json:"status"`
			Data   []platform.ServiceInfo `json:"data"`
		}
		if err := protocol.DecodePayload(env, &result); err != nil || result.Status != "success" {
			return false, "", "", nil
		}
		for _, svc := range result.Data {
			if svc.Name == cfg.ServiceName {
				if svc.Status != "running" {
					return true, "critical", fmt.Sprintf("service %s is %s", cfg.ServiceName, svc.Status), nil
				}
				return false, "", "", nil
			}
		}
		return true, "critical", fmt.Sprintf("service %s not found", cfg.ServiceName), nil

	default:
		return false, "", "", nil
	}
}

// RunSweeper periodically evaluates all rules until ctx is cancelled.
func RunSweeper(ctx context.Context, engine *Engine, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := engine.Evaluate(ctx); err != nil {
				slog.Error("alert evaluation sweep failed", "error", err)
			}
		}
	}
}
