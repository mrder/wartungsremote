package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
	"wartungsremote/internal/protocol"
	"wartungsremote/internal/remotesession"
)

const terminalSessionTTL = 30 * time.Minute

type createSessionRequest struct {
	Kind string `json:"kind"`
}

func (h *handlers) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteTerminal)
	if !ok {
		return
	}
	var req createSessionRequest
	if err := decodeJSON(r, &req); err != nil || req.Kind == "" {
		req.Kind = "terminal"
	}
	if req.Kind != "terminal" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "unsupported session kind")
		return
	}
	if !h.hub.IsOnline(d.ID) {
		writeErr(w, http.StatusConflict, "device_busy", "Device is not currently connected")
		return
	}

	user, _ := authpkg.UserFromContext(r.Context())
	sess, err := h.sessions.OpenTerminal(r.Context(), d.ID, user.ID, d.CustomerID, terminalSessionTTL)
	if err != nil {
		code := "internal_error"
		msg := "Failed to open remote session"
		switch {
		case errors.Is(err, remotesession.ErrDeviceOffline):
			code, msg = "device_busy", "Device is not currently connected"
		case errors.Is(err, remotesession.ErrAgentRejected):
			code, msg = "agent_rejected", agentRejectionReason(err)
		}
		writeErr(w, http.StatusBadGateway, code, msg)
		return
	}

	h.broker.Register(sess.ID, d.ID, protocol.StreamKindTerminal)

	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID, SessionID: &sess.ID,
		EventType: "remote_session.opened", Result: audit.ResultSuccess, SourceIP: clientIP(r),
		Metadata: map[string]any{"kind": sess.Kind},
	})

	writeJSON(w, http.StatusCreated, map[string]any{
		"session_id": sess.ID,
		"state":      sess.State,
		"expires_at": sess.ExpiresAt,
	}, nil)
}

// loadSessionWithAccess loads a remote session and verifies the caller owns
// it (or holds device.manage as an administrative override), matching
// docs/STATE_MACHINES.md §3 session ownership semantics.
func (h *handlers) loadSessionWithAccess(w http.ResponseWriter, r *http.Request) (remotesession.Session, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid session id")
		return remotesession.Session{}, false
	}
	sess, err := h.sessionRepo.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "resource_not_found", "Session not found")
		return remotesession.Session{}, false
	}
	user, _ := authpkg.UserFromContext(r.Context())
	grants := authpkg.GrantsFromContext(r.Context())
	d, derr := h.devices.GetByID(r.Context(), sess.DeviceID)
	isOwner := sess.UserID == user.ID
	isAdmin := derr == nil && authpkg.HasPermission(grants, authpkg.PermDeviceManage, deviceResource(d))
	if !isOwner && !isAdmin {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return remotesession.Session{}, false
	}
	return sess, true
}

func (h *handlers) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.loadSessionWithAccess(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sess.ID, "state": sess.State, "expires_at": sess.ExpiresAt, "kind": sess.Kind,
	}, nil)
}

func (h *handlers) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.loadSessionWithAccess(w, r)
	if !ok {
		return
	}
	// Idempotent: a session may already have been closed by the browser's
	// WebSocket disconnecting (see handleSessionStream) before this
	// explicit close request lands — don't double-audit or re-notify.
	if sess.State != remotesession.StateActive {
		writeJSON(w, http.StatusOK, map[string]any{"state": "closed"}, nil)
		return
	}
	_ = h.sessions.Close(r.Context(), sess, "admin_closed")
	h.broker.Unregister(sess.ID)

	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &sess.DeviceID, SessionID: &sess.ID,
		EventType: "remote_session.closed", Result: audit.ResultSuccess, SourceIP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "closed"}, nil)
}

// handleSessionStream is the browser-facing WebSocket for an active
// terminal session: binary frames carry raw terminal I/O; text frames carry
// small JSON control messages (currently just resize).
func (h *handlers) handleSessionStream(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.loadSessionWithAccess(w, r)
	if !ok {
		return
	}
	if sess.State != remotesession.StateActive {
		writeErr(w, http.StatusConflict, "session_expired", "Session is not active")
		return
	}
	stream, ok := h.broker.Get(sess.ID)
	if !ok {
		writeErr(w, http.StatusGone, "resource_not_found", "Stream no longer available")
		return
	}

	// coder/websocket's default Origin check requires Origin == Host, which
	// legitimately fails behind any reverse proxy that terminates the SPA
	// and the API on different origins (including this project's own dev
	// setup: Vite on :5173 proxying to wr-core on :9443). The real
	// authorization boundary for this endpoint is already enforced above
	// (SameSite=Strict session cookie + loadSessionWithAccess permission
	// check), so Origin matching here is intentionally permissive rather
	// than silently broken.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(int64(protocol.MaxTerminalControlBytes) * 8) // generous headroom for bursty terminal output

	deadlineCtx, cancelDeadline := context.WithDeadline(r.Context(), sess.ExpiresAt)
	defer cancelDeadline()
	// ctx is additionally cancelled the instant the read loop below exits,
	// so the write goroutine wakes up immediately instead of blocking
	// forever on an agent that has nothing more to send — without this, a
	// client-initiated close (the common case) never unblocks the other
	// goroutine, `done` never closes, and cleanup (broker.Unregister,
	// marking the session closed) never runs.
	ctx, cancel := context.WithCancel(deadlineCtx)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case payload, more := <-stream.FromAgent():
				if !more {
					_ = conn.Close(websocket.StatusNormalClosure, "session_closed")
					return
				}
				if err := conn.Write(ctx, websocket.MessageBinary, payload); err != nil {
					return
				}
			case <-ctx.Done():
				_ = conn.Close(websocket.StatusNormalClosure, "session_expired")
				return
			}
		}
	}()

	for {
		msgType, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		switch msgType {
		case websocket.MessageBinary:
			_ = stream.Send(data)
		case websocket.MessageText:
			var ctrl struct {
				Cols int `json:"cols"`
				Rows int `json:"rows"`
			}
			if json.Unmarshal(data, &ctrl) == nil && ctrl.Cols > 0 && ctrl.Rows > 0 {
				_ = h.sessions.Resize(ctx, sess, ctrl.Cols, ctrl.Rows)
			}
		}
	}
	cancel()

	<-done
	h.broker.Unregister(sess.ID)
	// Idempotent: an explicit DELETE /sessions/:id (or an agent-initiated
	// close) may have already closed this session while the WS loop above
	// was still draining.
	if current, err := h.sessionRepo.Get(context.Background(), sess.ID); err == nil && current.State == remotesession.StateActive {
		_ = h.sessions.Close(context.Background(), sess, "browser_disconnected")
		_ = h.audit.Record(context.Background(), audit.Event{
			ActorType: audit.ActorUser, ActorID: &sess.UserID, DeviceID: &sess.DeviceID, SessionID: &sess.ID,
			EventType: "remote_session.closed", Result: audit.ResultSuccess, Metadata: map[string]any{"reason": "browser_disconnected"},
		})
	}
}

type privilegeRequest struct {
	ReauthID        string `json:"reauth_id"`
	DurationSeconds int    `json:"duration_seconds"`
}

func (h *handlers) handleGrantPrivilege(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.loadSessionWithAccess(w, r)
	if !ok {
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	grants := authpkg.GrantsFromContext(r.Context())
	d, err := h.devices.GetByID(r.Context(), sess.DeviceID)
	if err != nil || !authpkg.HasPermission(grants, authpkg.PermPrivilegeRequest, deviceResource(d)) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}

	var req privilegeRequest
	if err := decodeJSON(r, &req); err != nil || req.ReauthID == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "reauth_id is required")
		return
	}
	valid, err := h.auth.ConsumeReauth(r.Context(), user.ID, req.ReauthID)
	if err != nil || !valid {
		writeErr(w, http.StatusForbidden, "privilege_required", "Reauthentication required")
		return
	}

	priv, err := h.sessions.GrantPrivilege(r.Context(), h.privilege, sess, time.Duration(req.DurationSeconds)*time.Second, h.privilegeTTL, "admin_requested")
	if err != nil {
		writeErr(w, http.StatusConflict, "invalid_request", "Session is not active")
		return
	}

	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &sess.DeviceID, SessionID: &sess.ID,
		EventType: "privilege.granted", Result: audit.ResultSuccess, SourceIP: clientIP(r),
		Metadata: map[string]any{"privilege_session_id": priv.ID, "valid_until": priv.ValidUntil},
	})
	writeJSON(w, http.StatusOK, map[string]any{"privilege_session_id": priv.ID, "valid_until": priv.ValidUntil}, nil)
}

func (h *handlers) handleRevokePrivilege(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.loadSessionWithAccess(w, r)
	if !ok {
		return
	}
	priv, err := h.privilege.ActiveForSession(r.Context(), sess.ID)
	if err != nil || priv == nil {
		writeErr(w, http.StatusNotFound, "resource_not_found", "No active privilege session")
		return
	}
	if err := h.sessions.RevokePrivilege(r.Context(), h.privilege, *priv); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to revoke privilege")
		return
	}
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &sess.DeviceID, SessionID: &sess.ID,
		EventType: "privilege.revoked", Result: audit.ResultSuccess, SourceIP: clientIP(r),
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "revoked"}, nil)
}
