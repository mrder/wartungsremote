package httpapi

import (
	"crypto/ed25519"
	"net/http"

	"github.com/google/uuid"

	"wartungsremote/internal/agentrelease"
	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
	"wartungsremote/internal/protocol"
)

func (h *handlers) handleListReleases(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAgentUpdate) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	releases, err := h.releases.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load releases")
		return
	}
	writeJSON(w, http.StatusOK, releases, nil)
}

func (h *handlers) handleCreateRelease(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAgentUpdate) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	if len(h.cfg.Secrets.ReleasePublicKey) != ed25519.PublicKeySize {
		writeErr(w, http.StatusPreconditionFailed, "not_configured", "No trusted release public key configured (WR_RELEASE_PUBLIC_KEY_FILE)")
		return
	}
	var body struct {
		Version           string `json:"version"`
		OSFamily          string `json:"os_family"`
		Architecture      string `json:"architecture"`
		Channel           string `json:"channel"`
		ArtifactURL       string `json:"artifact_url"`
		ArtifactSHA256Hex string `json:"artifact_sha256_hex"`
		SignatureBase64   string `json:"signature_base64"`
		MinimumSupported  bool   `json:"minimum_supported"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	if body.Version == "" || body.OSFamily == "" || body.Architecture == "" || body.ArtifactURL == "" || body.ArtifactSHA256Hex == "" || body.SignatureBase64 == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "version, os_family, architecture, artifact_url, artifact_sha256_hex and signature_base64 are required")
		return
	}
	channel := body.Channel
	if channel == "" {
		channel = "stable"
	}

	rl, err := h.releases.Create(r.Context(), agentrelease.Release{
		Version: body.Version, OSFamily: body.OSFamily, Architecture: body.Architecture, Channel: channel,
		ArtifactURL: body.ArtifactURL, ArtifactSHA256Hex: body.ArtifactSHA256Hex, SignatureBase64: body.SignatureBase64,
		MinimumSupported: body.MinimumSupported,
	}, ed25519.PublicKey(h.cfg.Secrets.ReleasePublicKey))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Release rejected: "+err.Error())
		return
	}

	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "agent_release.created", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"version": rl.Version, "os_family": rl.OSFamily, "architecture": rl.Architecture, "channel": rl.Channel},
	})
	writeJSON(w, http.StatusCreated, rl, nil)
}

func (h *handlers) handleSetReleaseBlocked(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermAgentUpdate) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid release id")
		return
	}
	var body struct {
		Blocked bool `json:"blocked"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	if err := h.releases.SetBlocked(r.Context(), id, body.Blocked); err != nil {
		if err == agentrelease.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "Release not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to update release")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleTriggerDeviceUpdate(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermAgentUpdate)
	if !ok {
		return
	}
	if !h.hub.IsOnline(d.ID) {
		writeErr(w, http.StatusConflict, "device_offline", "Device is not connected")
		return
	}
	var body struct {
		Channel string `json:"channel"`
	}
	_ = decodeJSON(r, &body)
	channel := body.Channel
	if channel == "" {
		channel = "stable"
	}

	rl, err := h.releases.Latest(r.Context(), d.OSFamily, d.Architecture, channel)
	if err != nil {
		if err == agentrelease.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "No matching release found for this device's OS/architecture/channel")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to look up releases")
		return
	}
	if rl.Version == d.AgentVersion {
		writeErr(w, http.StatusConflict, "already_current", "Device already runs this version")
		return
	}

	if err := h.hub.SendMessage(r.Context(), d.ID, protocol.TypeDeviceCommand, protocol.DeviceCommandPayload{
		CommandType: protocol.CmdAgentUpdate,
		Params: mustJSON(protocol.AgentUpdateParams{
			Version: rl.Version, ArtifactURL: rl.ArtifactURL,
			ArtifactSHA256Hex: rl.ArtifactSHA256Hex, SignatureBase64: rl.SignatureBase64,
		}),
	}); err != nil {
		writeErr(w, http.StatusConflict, "device_busy", "Failed to send update command to device")
		return
	}

	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID,
		EventType: "agent.update.triggered", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"target_version": rl.Version, "from_version": d.AgentVersion},
	})
	writeJSON(w, http.StatusOK, map[string]any{"target_version": rl.Version}, nil)
}
