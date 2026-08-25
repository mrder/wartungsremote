// Incident-response endpoints per docs/SECURITY.md §20: bulk enrollment
// token revocation, user account status + all-sessions revocation. (Single
// device credential revocation already exists as POST /devices/:id/revoke;
// agent-version blocking is enforced at the control-channel handshake via
// controlhub.Hub's VersionBlockedChecker, configured in router.go.)
package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
)

func (h *handlers) handleRevokeAllEnrollments(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermCredentialRevoke) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	n, err := h.enroll.RevokeAllOutstanding(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to revoke enrollment tokens")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "enrollment.revoked_all", Result: audit.ResultSuccess, SourceIP: clientIP(r),
		Metadata: map[string]any{"count": n},
	})
	writeJSON(w, http.StatusOK, map[string]any{"revoked_count": n}, nil)
}

func (h *handlers) handleListUsers(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermUserManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	users, err := h.auth.Repo.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to load users")
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, map[string]any{
			"id": u.ID, "username": u.Username, "display_name": u.DisplayName,
			"status": u.Status, "mfa_required": u.MFARequired,
			"created_at": u.CreatedAt, "last_login_at": u.LastLoginAt,
		})
	}
	writeJSON(w, http.StatusOK, out, nil)
}

func (h *handlers) handleSetUserStatus(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermUserManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid user id")
		return
	}
	var body struct {
		Status      *string `json:"status"`
		MFARequired *bool   `json:"mfa_required"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "Malformed request body")
		return
	}
	if body.Status == nil && body.MFARequired == nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "status or mfa_required is required")
		return
	}
	actor, _ := authpkg.UserFromContext(r.Context())

	if body.Status != nil {
		status := authpkg.UserStatus(*body.Status)
		switch status {
		case authpkg.UserActive, authpkg.UserDisabled, authpkg.UserLocked:
		default:
			writeErr(w, http.StatusBadRequest, "invalid_request", "status must be active, disabled or locked")
			return
		}
		if id == actor.ID && status != authpkg.UserActive {
			writeErr(w, http.StatusConflict, "invalid_request", "cannot disable/lock your own account")
			return
		}
		if err := h.auth.Repo.SetStatus(r.Context(), id, status); err != nil {
			if err == authpkg.ErrNotFound {
				writeErr(w, http.StatusNotFound, "not_found", "User not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to update user")
			return
		}
		// Changing status away from active must take effect immediately, not
		// just block future logins — kill whatever sessions are already open.
		if status != authpkg.UserActive {
			_ = h.auth.Sessions.RevokeAllForUser(r.Context(), id)
		}
		_ = h.audit.Record(r.Context(), audit.Event{
			ActorType: audit.ActorUser, ActorID: &actor.ID,
			EventType: "user.status_changed", Result: audit.ResultSuccess, SourceIP: clientIP(r),
			Metadata: map[string]any{"target_user_id": id, "status": string(status)},
		})
	}

	if body.MFARequired != nil {
		if err := h.auth.Repo.SetMFARequired(r.Context(), id, *body.MFARequired); err != nil {
			if err == authpkg.ErrNotFound {
				writeErr(w, http.StatusNotFound, "not_found", "User not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to update user")
			return
		}
		_ = h.audit.Record(r.Context(), audit.Event{
			ActorType: audit.ActorUser, ActorID: &actor.ID,
			EventType: "user.mfa_required_changed", Result: audit.ResultSuccess, SourceIP: clientIP(r),
			Metadata: map[string]any{"target_user_id": id, "mfa_required": *body.MFARequired},
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasAnyGrant(grants, authpkg.PermUserManage) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid user id")
		return
	}
	if err := h.auth.Sessions.RevokeAllForUser(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to revoke sessions")
		return
	}
	actor, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &actor.ID,
		EventType: "user.sessions_revoked", Result: audit.ResultSuccess, SourceIP: clientIP(r),
		Metadata: map[string]any{"target_user_id": id},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}
