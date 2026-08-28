package httpapi

import (
	"errors"
	"net/http"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *handlers) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil || req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "username and password are required")
		return
	}

	res, err := h.auth.Login(r.Context(), req.Username, req.Password, h.clientIP(r), r.UserAgent(), h.cfg.Admin.RequireMFA)
	if err != nil {
		h.auditLoginFailure(r, req.Username, err)
		switch {
		case errors.Is(err, authpkg.ErrRateLimited):
			writeErr(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again later")
		case errors.Is(err, authpkg.ErrLocked):
			writeErr(w, http.StatusForbidden, "permission_denied", "Account temporarily locked")
		default:
			writeErr(w, http.StatusUnauthorized, "unauthenticated", "Invalid username or password")
		}
		return
	}

	switch res.State {
	case "mfa_setup_required":
		_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, EventType: audit.EventLoginSuccess, Result: audit.ResultSuccess, SourceIP: h.clientIP(r), Metadata: map[string]any{"stage": "mfa_setup_required", "username": req.Username}})
		writeJSON(w, http.StatusOK, map[string]any{"state": "mfa_setup_required", "setup_uri": res.SetupURI}, nil)
	case "mfa_required":
		writeJSON(w, http.StatusOK, map[string]any{"state": "mfa_required", "challenge_id": res.ChallengeID}, nil)
	case "authenticated":
		h.auth.Sessions.SetCookie(w, res.Token, res.Session.ExpiresAt)
		setCSRFCookie(w, h.cfg)
		_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, ActorID: &res.Session.UserID, SessionID: &res.Session.ID, EventType: audit.EventLoginSuccess, Result: audit.ResultSuccess, SourceIP: h.clientIP(r)})
		writeJSON(w, http.StatusOK, map[string]any{"state": "authenticated"}, nil)
	}
}

func (h *handlers) auditLoginFailure(r *http.Request, username string, err error) {
	eventType := audit.EventLoginFailure
	if errors.Is(err, authpkg.ErrLocked) {
		eventType = audit.EventLoginLockout
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser,
		EventType: eventType,
		Result:    audit.ResultFailure,
		SourceIP:  h.clientIP(r),
		Metadata:  map[string]any{"username": username},
	})
}

type totpRequest struct {
	ChallengeID string `json:"challenge_id"`
	Code        string `json:"code"`
}

func (h *handlers) handleTOTP(w http.ResponseWriter, r *http.Request) {
	var req totpRequest
	if err := decodeJSON(r, &req); err != nil || req.ChallengeID == "" || req.Code == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "challenge_id and code are required")
		return
	}
	res, err := h.auth.CompleteMFA(r.Context(), req.ChallengeID, req.Code, h.clientIP(r), r.UserAgent())
	if err != nil {
		_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, EventType: audit.EventMFAChallengeFailure, Result: audit.ResultFailure, SourceIP: h.clientIP(r)})
		if errors.Is(err, authpkg.ErrRateLimited) {
			writeErr(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again later")
			return
		}
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "Invalid or expired code")
		return
	}
	h.auth.Sessions.SetCookie(w, res.Token, res.Session.ExpiresAt)
	setCSRFCookie(w, h.cfg)
	_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, ActorID: &res.Session.UserID, SessionID: &res.Session.ID, EventType: audit.EventMFAChallengeSuccess, Result: audit.ResultSuccess, SourceIP: h.clientIP(r)})
	writeJSON(w, http.StatusOK, map[string]any{"state": "authenticated"}, nil)
}

type mfaSetupConfirmRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

// handleMFASetupConfirm finalizes first-time TOTP enrollment. It re-verifies
// the password in this same request because no session exists yet at this
// point in the flow (login previously returned mfa_setup_required instead of
// a session).
func (h *handlers) handleMFASetupConfirm(w http.ResponseWriter, r *http.Request) {
	var req mfaSetupConfirmRequest
	if err := decodeJSON(r, &req); err != nil || req.Username == "" || req.Password == "" || req.Code == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "username, password and code are required")
		return
	}
	if !h.auth.LoginLimiter.Allow("mfa-setup:" + h.clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again later")
		return
	}
	user, err := h.auth.Repo.GetByUsername(r.Context(), req.Username)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "Invalid credentials")
		return
	}
	ok, err := authpkg.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !ok {
		_ = h.auth.Repo.RegisterFailedLogin(r.Context(), user.ID)
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "Invalid credentials")
		return
	}

	codes, err := h.auth.ConfirmMFASetup(r.Context(), user.ID, req.Code)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "Invalid code")
		return
	}
	_ = h.auth.Repo.ResetFailedLogins(r.Context(), user.ID)
	token, sess, err := h.auth.Sessions.Create(r.Context(), user.ID, h.clientIP(r), r.UserAgent())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to create session")
		return
	}
	h.auth.Sessions.SetCookie(w, token, sess.ExpiresAt)
	setCSRFCookie(w, h.cfg)
	_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, ActorID: &user.ID, SessionID: &sess.ID, EventType: "auth.mfa.setup_confirmed", Result: audit.ResultSuccess, SourceIP: h.clientIP(r)})
	writeJSON(w, http.StatusOK, map[string]any{"state": "authenticated", "recovery_codes": codes}, nil)
}

func (h *handlers) handleLogout(w http.ResponseWriter, r *http.Request) {
	token, err := h.auth.Sessions.ReadCookie(r)
	if err == nil {
		if sess, err := h.auth.Sessions.Validate(r.Context(), token); err == nil {
			_ = h.auth.Sessions.Revoke(r.Context(), sess.ID)
			_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, ActorID: &sess.UserID, SessionID: &sess.ID, EventType: audit.EventLogout, Result: audit.ResultSuccess, SourceIP: h.clientIP(r)})
		}
	}
	h.auth.Sessions.ClearCookie(w)
	clearCSRFCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"state": "logged_out"}, nil)
}

type reauthRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (h *handlers) handleReauth(w http.ResponseWriter, r *http.Request) {
	token, err := h.auth.Sessions.ReadCookie(r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "No active session")
		return
	}
	sess, err := h.auth.Sessions.Validate(r.Context(), token)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "Session invalid or expired")
		return
	}
	var req reauthRequest
	if err := decodeJSON(r, &req); err != nil || req.Password == "" || req.Code == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "password and code are required")
		return
	}
	reauthID, err := h.auth.Reauth(r.Context(), sess.UserID, req.Password, req.Code, h.clientIP(r))
	if err != nil {
		_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, ActorID: &sess.UserID, SessionID: &sess.ID, EventType: audit.EventReauthFailure, Result: audit.ResultFailure, SourceIP: h.clientIP(r)})
		if errors.Is(err, authpkg.ErrRateLimited) {
			writeErr(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts, try again later")
			return
		}
		writeErr(w, http.StatusUnauthorized, "unauthenticated", "Invalid credentials")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{ActorType: audit.ActorUser, ActorID: &sess.UserID, SessionID: &sess.ID, EventType: audit.EventReauthSuccess, Result: audit.ResultSuccess, SourceIP: h.clientIP(r)})
	writeJSON(w, http.StatusOK, map[string]any{"reauth_id": reauthID}, nil)
}

type changePasswordRequest struct {
	ReauthID    string `json:"reauth_id"`
	NewPassword string `json:"new_password"`
}

// handleChangePassword is self-service only — a user changing their own
// password. It never touches other sessions (unlike an admin disabling a
// suspected-compromised account elsewhere), since this is an intentional
// action by the account owner, not incident response. Requires a fresh
// reauth_id (current password + MFA, via /auth/reauth) exactly like
// privilege elevation does — changing your own password is just as
// sensitive.
func (h *handlers) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user, _ := authpkg.UserFromContext(r.Context())
	var req changePasswordRequest
	if err := decodeJSON(r, &req); err != nil || req.ReauthID == "" || req.NewPassword == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "reauth_id and new_password are required")
		return
	}
	valid, err := h.auth.ConsumeReauth(r.Context(), user.ID, req.ReauthID)
	if err != nil || !valid {
		writeErr(w, http.StatusForbidden, "privilege_required", "Reauthentication required")
		return
	}
	if err := h.auth.ChangeOwnPassword(r.Context(), user.ID, req.NewPassword); err != nil {
		if errors.Is(err, authpkg.ErrPasswordTooShort) {
			writeErr(w, http.StatusBadRequest, "invalid_request", "Password must be at least 12 characters")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to change password")
		return
	}
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID,
		EventType: "user.password_changed", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleMe(w http.ResponseWriter, r *http.Request) {
	user, _ := authpkg.UserFromContext(r.Context())
	sess, _ := authpkg.SessionFromContext(r.Context())
	grants := authpkg.GrantsFromContext(r.Context())

	permissions := make([]string, 0, len(grants))
	seen := map[string]bool{}
	for _, g := range grants {
		if !seen[g.Permission] {
			seen[g.Permission] = true
			permissions = append(permissions, g.Permission)
		}
	}
	confirmed, _ := h.auth.MFA.IsConfirmed(r.Context(), user.ID)

	writeJSON(w, http.StatusOK, map[string]any{
		"id":                 user.ID,
		"username":           user.Username,
		"display_name":       user.DisplayName,
		"permissions":        permissions,
		"mfa_confirmed":      confirmed,
		"session_expires_at": sess.ExpiresAt,
		// Needed by the web UI to show the exact `wr-helper --server ...`
		// command for SSH/RDP tunnels (docs/RELAY.md §5) — not a secret.
		"public_base_url": h.cfg.Public.BaseURL,
	}, nil)
}
