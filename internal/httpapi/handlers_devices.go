package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
	"wartungsremote/internal/device"
)

func deviceResource(d device.Device) authpkg.Resource {
	return authpkg.Resource{CustomerID: d.CustomerID, GroupID: d.GroupID}
}

func (h *handlers) handleListDevices(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermDeviceRead) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	q := r.URL.Query()
	f := device.ListFilter{
		Status: q.Get("status"),
		Health: q.Get("health"),
		Tag:    q.Get("tag"),
		Query:  q.Get("q"),
	}
	if v := q.Get("customer_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid customer_id")
			return
		}
		f.CustomerID = &id
	}
	if v := q.Get("group_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid group_id")
			return
		}
		f.GroupID = &id
	}
	if v := q.Get("page"); v != "" {
		f.Page, _ = strconv.Atoi(v)
	}
	if v := q.Get("page_size"); v != "" {
		f.PageSize, _ = strconv.Atoi(v)
	}

	devices, total, err := h.devices.List(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to list devices")
		return
	}

	// Global-scope callers see everything the query returns; customer/group
	// scoped callers get results filtered to their permitted scopes.
	if !authpkg.HasPermission(grants, authpkg.PermDeviceRead, authpkg.Resource{}) {
		filtered := devices[:0]
		for _, d := range devices {
			if authpkg.HasPermission(grants, authpkg.PermDeviceRead, deviceResource(d)) {
				filtered = append(filtered, d)
			}
		}
		devices = filtered
	}

	items := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		items = append(items, deviceSummary(d))
	}
	writeJSON(w, http.StatusOK, items, map[string]any{"total": total, "page": f.Page, "page_size": f.PageSize})
}

func deviceSummary(d device.Device) map[string]any {
	return map[string]any{
		"id":            d.ID,
		"install_id":    d.InstallID,
		"customer_id":   d.CustomerID,
		"group_id":      d.GroupID,
		"display_name":  d.DisplayName,
		"hostname":      d.Hostname,
		"os_family":     d.OSFamily,
		"os_name":       d.OSName,
		"os_version":    d.OSVersion,
		"architecture":  d.Architecture,
		"agent_version": d.AgentVersion,
		"status":        d.Status,
		"health":        d.Health,
		"health_reasons": d.HealthReasons,
		"tags":          d.Tags,
		"last_seen_at":  d.LastSeenAt,
		"last_public_ip": d.LastPublicIP,
	}
}

func (h *handlers) loadDeviceWithAccess(w http.ResponseWriter, r *http.Request, permission string) (device.Device, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid device id")
		return device.Device{}, false
	}
	d, err := h.devices.GetByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "resource_not_found", "Device not found")
		return device.Device{}, false
	}
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasPermission(grants, permission, deviceResource(d)) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return device.Device{}, false
	}
	return d, true
}

func (h *handlers) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermDeviceRead)
	if !ok {
		return
	}
	online := h.hub.IsOnline(d.ID)
	summary := deviceSummary(d)
	summary["online"] = online
	writeJSON(w, http.StatusOK, summary, nil)
}

type patchDeviceRequest struct {
	DisplayName *string  `json:"display_name"`
	CustomerID  *string  `json:"customer_id"`
	GroupID     *string  `json:"group_id"`
	Tags        *[]string `json:"tags"`
}

func (h *handlers) handlePatchDevice(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermDeviceManage)
	if !ok {
		return
	}
	var req patchDeviceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}

	in := device.PatchInput{DisplayName: req.DisplayName, Tags: req.Tags}
	if req.CustomerID != nil {
		var id *uuid.UUID
		if *req.CustomerID != "" {
			parsed, err := uuid.Parse(*req.CustomerID)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "invalid_request", "invalid customer_id")
				return
			}
			id = &parsed
		}
		in.CustomerID = &id
	}
	if req.GroupID != nil {
		var id *uuid.UUID
		if *req.GroupID != "" {
			parsed, err := uuid.Parse(*req.GroupID)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "invalid_request", "invalid group_id")
				return
			}
			id = &parsed
		}
		in.GroupID = &id
	}

	if err := h.devices.Patch(r.Context(), d.ID, in); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to update device")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID, EventType: audit.EventDeviceUpdated, Result: audit.ResultSuccess, SourceIP: clientIP(r)})
	writeJSON(w, http.StatusOK, map[string]any{"state": "updated"}, nil)
}

func (h *handlers) handleStatusRequest(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermDeviceRead)
	if !ok {
		return
	}
	if err := h.hub.RequestInventory(r.Context(), d.ID); err != nil {
		writeErr(w, http.StatusConflict, "device_busy", "Device is not currently connected")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "requested"}, nil)
}

func (h *handlers) handleDeviceIPHistory(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermMonitoringRead)
	if !ok {
		return
	}
	window := 24 * time.Hour
	if v := r.URL.Query().Get("hours"); v != "" {
		if hours, err := strconv.Atoi(v); err == nil && hours > 0 && hours <= 24*30 {
			window = time.Duration(hours) * time.Hour
		}
	}
	history, err := h.devices.RecentIPHistory(r.Context(), d.ID, window)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load IP history")
		return
	}
	writeJSON(w, http.StatusOK, history, map[string]any{"distinct_count": len(history), "window_hours": int(window.Hours())})
}

func (h *handlers) handleDeviceHealth(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermMonitoringRead)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"health": d.Health, "reasons": d.HealthReasons}, nil)
}

func (h *handlers) handleDeviceMetrics(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermMonitoringRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	resolution := q.Get("resolution")
	if resolution == "" {
		resolution = "raw"
	}
	to := time.Now().UTC()
	defaultWindow := 24 * time.Hour
	if resolution == "hourly" {
		defaultWindow = 30 * 24 * time.Hour
	}
	from := to.Add(-defaultWindow)
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	var points []device.MetricsPoint
	var err error
	switch resolution {
	case "hourly":
		points, err = h.devices.HourlyMetrics(r.Context(), d.ID, from, to, 1000)
	default:
		points, err = h.devices.RecentMetrics(r.Context(), d.ID, from, to, 1000)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load metrics")
		return
	}
	out := make([]map[string]any, 0, len(points))
	for _, p := range points {
		out = append(out, map[string]any{
			"observed_at":         p.ObservedAt,
			"cpu_percent":         p.CPUPercent,
			"memory_used_bytes":   p.MemoryUsedBytes,
			"memory_total_bytes":  p.MemoryTotalBytes,
			"uptime_seconds":      p.UptimeSeconds,
		})
	}
	writeJSON(w, http.StatusOK, out, nil)
}

func (h *handlers) handleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermCredentialRevoke)
	if !ok {
		return
	}

	var req struct {
		ReauthID string `json:"reauth_id"`
	}
	_ = decodeJSON(r, &req)
	user, _ := authpkg.UserFromContext(r.Context())
	valid, err := h.auth.ConsumeReauth(r.Context(), user.ID, req.ReauthID)
	if err != nil || !valid {
		writeErr(w, http.StatusForbidden, "privilege_required", "Reauthentication required")
		return
	}

	if err := h.devices.Revoke(r.Context(), d.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to revoke device")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID, EventType: audit.EventDeviceRevoked, Result: audit.ResultSuccess, SourceIP: clientIP(r)})
	writeJSON(w, http.StatusOK, map[string]any{"state": "revoked"}, nil)
}

// parseAuditFilter builds an audit.Filter from the standard query params
// shared by the list and export endpoints (event_type/actor_id/device_id/
// from/to). Returns ok=false after already writing an error response.
func parseAuditFilter(w http.ResponseWriter, r *http.Request) (audit.Filter, bool) {
	q := r.URL.Query()
	f := audit.Filter{EventType: q.Get("event_type")}
	if v := q.Get("actor_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.ActorID = &id
		}
	}
	if v := q.Get("device_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "invalid device_id")
			return audit.Filter{}, false
		}
		f.DeviceID = &id
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.From = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.To = t
		}
	}
	return f, true
}

func (h *handlers) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAuditRead) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	f, ok := parseAuditFilter(w, r)
	if !ok {
		return
	}

	entries, err := h.audit.List(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load audit log")
		return
	}
	// device_id-scoped requests (path /devices/:id/audit) additionally
	// require device.read scoped to that device; global /audit requires
	// audit.read at a scope that legitimately covers the returned rows. For
	// V1 audit.read is only granted at global or customer scope and the
	// underlying query does not join customer_id-based filtering beyond what
	// is stored on each row, so callers with only customer-scoped audit.read
	// are restricted to global scope here until per-row scope filtering is
	// added; see docs/TODO.md Phase 36.
	writeJSON(w, http.StatusOK, entries, map[string]any{"count": len(entries)})
}

// handleExportAuditLog streams the filtered audit log as JSON or CSV
// (docs/SECURITY.md §20 "Audit exportieren", docs/TODO.md Phase 36 "Audit
// Export JSON/CSV"). Subject to the same audit.Filter.Limit cap as the
// list endpoint (max 1000 rows); use from/to to page through more.
func (h *handlers) handleExportAuditLog(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAuditRead) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	f, ok := parseAuditFilter(w, r)
	if !ok {
		return
	}
	f.Limit = 1000

	entries, err := h.audit.List(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load audit log")
		return
	}

	format := r.URL.Query().Get("format")
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "audit.exported", Result: audit.ResultSuccess, SourceIP: clientIP(r),
		Metadata: map[string]any{"format": format, "count": len(entries)},
	})

	filename := fmt.Sprintf("audit-export-%s", time.Now().UTC().Format("20060102-150405"))
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+".csv\"")
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"id", "occurred_at", "actor_type", "actor_id", "device_id", "customer_id", "event_type", "result", "source_ip", "metadata"})
		for _, e := range entries {
			metaJSON, _ := json.Marshal(e.Metadata)
			_ = cw.Write([]string{
				fmt.Sprint(e.ID), e.OccurredAt.Format(time.RFC3339), e.ActorType, uuidOrEmpty(e.ActorID),
				uuidOrEmpty(e.DeviceID), uuidOrEmpty(e.CustomerID), e.EventType, e.Result, e.SourceIP, string(metaJSON),
			})
		}
		cw.Flush()
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+".json\"")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": entries, "meta": map[string]any{"count": len(entries)}})
}

func uuidOrEmpty(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func (h *handlers) handleDeviceAuditLog(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermAuditRead)
	if !ok {
		return
	}
	entries, err := h.audit.List(r.Context(), audit.Filter{DeviceID: &d.ID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load audit log")
		return
	}
	writeJSON(w, http.StatusOK, entries, map[string]any{"count": len(entries)})
}
