package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
	"wartungsremote/internal/protocol"
	"wartungsremote/internal/support"
)

// handleGetSupportCredential returns the dedicated remote-support OS
// account's current username/password, decrypted on the fly. Every reveal
// is audited — this is sensitive, standing access to a device's SSH/RDP
// login, not a routine read. Either tunnel permission is sufficient: the
// credential exists to log into whichever tunnel (SSH or RDP) this device
// exposes.
func (h *handlers) handleGetSupportCredential(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid device id")
		return
	}
	d, err := h.devices.GetByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "resource_not_found", "Device not found")
		return
	}
	grants := authpkg.GrantsFromContext(r.Context())
	res := deviceResource(d)
	if !authpkg.HasPermission(grants, authpkg.PermRemoteTunnelSSH, res) && !authpkg.HasPermission(grants, authpkg.PermRemoteTunnelRDP, res) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}

	cred, err := h.support.Get(r.Context(), d.ID)
	if err != nil {
		if err == support.ErrNotFound {
			writeErr(w, http.StatusNotFound, "not_found", "No remote-support account has been reported by this device yet")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load credential")
		return
	}

	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID,
		EventType: "support_credential.revealed", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"username":   cred.Username,
		"password":   cred.Password,
		"updated_at": cred.UpdatedAt,
	}, nil)
}

// handleRotateSupportCredential tells the (must be online) agent to
// generate a fresh password for the remote-support account and apply it
// locally; the agent reports the new credential back over the control
// channel once done (protocol.TypeSupportCredentialReport), same as after
// initial provisioning.
func (h *handlers) handleRotateSupportCredential(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid device id")
		return
	}
	d, err := h.devices.GetByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "resource_not_found", "Device not found")
		return
	}
	grants := authpkg.GrantsFromContext(r.Context())
	res := deviceResource(d)
	if !authpkg.HasPermission(grants, authpkg.PermRemoteTunnelSSH, res) && !authpkg.HasPermission(grants, authpkg.PermRemoteTunnelRDP, res) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	if !h.hub.IsOnline(d.ID) {
		writeErr(w, http.StatusConflict, "device_offline", "Device is not connected")
		return
	}
	if err := h.hub.SendMessage(r.Context(), d.ID, protocol.TypeDeviceCommand, protocol.DeviceCommandPayload{
		CommandType: protocol.CmdRotateSupportCredential,
	}); err != nil {
		writeErr(w, http.StatusConflict, "device_busy", "Failed to send rotate command to device")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID,
		EventType: "support_credential.rotate_requested", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}
