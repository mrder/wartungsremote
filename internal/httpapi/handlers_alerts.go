package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"wartungsremote/internal/alerting"
	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
)

func (h *handlers) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermMonitoringRead) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	rules, err := h.alerts.ListRules(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load alert rules")
		return
	}
	writeJSON(w, http.StatusOK, rules, nil)
}

func (h *handlers) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAlertManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	var body struct {
		ScopeType string          `json:"scope_type"`
		ScopeID   *uuid.UUID      `json:"scope_id"`
		RuleType  string          `json:"rule_type"`
		Config    json.RawMessage `json:"config"`
		Enabled   *bool           `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	switch body.ScopeType {
	case alerting.ScopeGlobal, alerting.ScopeCustomer, alerting.ScopeGroup, alerting.ScopeDevice:
	default:
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid scope_type")
		return
	}
	if body.ScopeType != alerting.ScopeGlobal && body.ScopeID == nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "scope_id required for non-global scope")
		return
	}
	switch body.RuleType {
	case alerting.RuleOffline, alerting.RuleCPU, alerting.RuleRAM, alerting.RuleDisk, alerting.RuleService, alerting.RuleAgentVersion:
	default:
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid rule_type")
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	rule, err := h.alerts.CreateRule(r.Context(), alerting.Rule{
		ScopeType: body.ScopeType, ScopeID: body.ScopeID, RuleType: body.RuleType, Config: body.Config, Enabled: enabled,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to create alert rule")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "alert_rule.created", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"rule_type": body.RuleType, "scope_type": body.ScopeType},
	})
	writeJSON(w, http.StatusCreated, rule, nil)
}

func (h *handlers) handleSetAlertRuleEnabled(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAlertManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid rule id")
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	if err := h.alerts.SetRuleEnabled(r.Context(), id, body.Enabled); err != nil {
		if err == alerting.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "Alert rule not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to update alert rule")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAlertManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid rule id")
		return
	}
	if err := h.alerts.DeleteRule(r.Context(), id); err != nil {
		if err == alerting.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "Alert rule not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to delete alert rule")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermMonitoringRead) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	q := r.URL.Query()
	f := alerting.AlertFilter{State: q.Get("state")}
	if v := q.Get("device_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			f.DeviceID = &id
		}
	}
	alerts, err := h.alerts.ListAlerts(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load alerts")
		return
	}
	writeJSON(w, http.StatusOK, alerts, nil)
}

func (h *handlers) handleAlertsOpenCount(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermMonitoringRead) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	n, err := h.alerts.CountOpen(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to count alerts")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"open_count": n}, nil)
}

func (h *handlers) handleAcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAlertManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid alert id")
		return
	}
	if err := h.alerts.Acknowledge(r.Context(), id); err != nil {
		if err == alerting.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "Open alert not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to acknowledge alert")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "alert.acknowledged", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"alert_id": id},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleResolveAlert(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAlertManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid alert id")
		return
	}
	if err := h.alerts.Resolve(r.Context(), id); err != nil {
		if err == alerting.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "Alert not found or already resolved")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to resolve alert")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "alert.resolved", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"alert_id": id, "reason": "manual"},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleDeleteAlert(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAlertManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid alert id")
		return
	}
	if err := h.alerts.Delete(r.Context(), id); err != nil {
		if err == alerting.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "Alert not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to delete alert")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "alert.deleted", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"alert_id": id},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}
