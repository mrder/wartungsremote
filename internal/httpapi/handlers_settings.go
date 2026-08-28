package httpapi

import (
	"net/http"
	"time"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
	"wartungsremote/internal/notify"
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

// handleGetTelegramSettings never returns the bot token — it's write-only
// from the API's perspective, same reasoning as any other secret. Re-enter
// it to change it.
func (h *handlers) handleGetTelegramSettings(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermSystemSettings) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	status, err := h.telegram.GetStatus(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured": status.Configured,
		"chat_id":    status.ChatID,
		"updated_at": status.UpdatedAt,
	}, nil)
}

func (h *handlers) handleSetTelegramSettings(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermSystemSettings) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	var body struct {
		BotToken string `json:"bot_token"`
		ChatID   string `json:"chat_id"`
	}
	if err := decodeJSON(r, &body); err != nil || body.BotToken == "" || body.ChatID == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "bot_token and chat_id are required")
		return
	}
	if err := h.telegram.Set(r.Context(), body.BotToken, body.ChatID); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to save settings")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "settings.telegram_configured", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

// handleTestTelegramSettings sends a test message using the *currently
// saved* configuration (save first, then test) so the dashboard can
// confirm a bot token/chat ID actually works before relying on it for
// real alerts.
func (h *handlers) handleTestTelegramSettings(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermSystemSettings) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	err := h.telegram.SendMessage(r.Context(), "✅ WartungsRemote: this is a test message. Your Telegram alert notifications are set up correctly.")
	if err != nil {
		if err == notify.ErrNotConfigured {
			writeErr(w, http.StatusNotFound, "not_found", "Telegram is not configured yet — save a bot token and chat ID first")
			return
		}
		writeErr(w, http.StatusBadGateway, "internal_error", "Failed to send test message: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}
