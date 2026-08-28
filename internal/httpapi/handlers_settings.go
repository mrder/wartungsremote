package httpapi

import (
	"net/http"
	"time"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
)

// handleGetRetentionSettings returns the effective metrics retention
// windows — the runtime override if one has been set, otherwise the
// server.yaml default (internal/appsettings.MetricsRetention).
func (h *handlers) handleGetRetentionSettings(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermSystemSettings) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	raw, hourly, err := h.settings.MetricsRetention(r.Context(), h.cfg.Metrics.RawRetention, h.cfg.Metrics.HourlyRetention)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"raw_retention_hours":    int(raw.Hours()),
		"hourly_retention_hours": int(hourly.Hours()),
	}, nil)
}

func (h *handlers) handleSetRetentionSettings(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermSystemSettings) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	var body struct {
		RawRetentionHours    int `json:"raw_retention_hours"`
		HourlyRetentionHours int `json:"hourly_retention_hours"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	if body.RawRetentionHours <= 0 || body.HourlyRetentionHours <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Both retention values must be positive")
		return
	}
	raw := time.Duration(body.RawRetentionHours) * time.Hour
	hourly := time.Duration(body.HourlyRetentionHours) * time.Hour
	if err := h.settings.SetMetricsRetention(r.Context(), raw, hourly); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to update settings")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "settings.retention_changed", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"raw_retention_hours": body.RawRetentionHours, "hourly_retention_hours": body.HourlyRetentionHours},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

// handleGetSupportCredentialRotationSettings returns the configured
// automatic rotation interval for remote-support account passwords (0 =
// disabled, the default).
func (h *handlers) handleGetSupportCredentialRotationSettings(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermSystemSettings) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	days, err := h.settings.SupportCredentialRotationDays(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rotation_days": days}, nil)
}

func (h *handlers) handleSetSupportCredentialRotationSettings(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermSystemSettings) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	var body struct {
		RotationDays int `json:"rotation_days"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	if body.RotationDays < 0 {
		writeErr(w, http.StatusBadRequest, "invalid_request", "rotation_days must be >= 0 (0 disables it)")
		return
	}
	if err := h.settings.SetSupportCredentialRotationDays(r.Context(), body.RotationDays); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to update settings")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "settings.support_credential_rotation_changed", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"rotation_days": body.RotationDays},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}
