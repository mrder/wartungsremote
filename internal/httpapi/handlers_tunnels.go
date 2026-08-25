package httpapi

import (
	"context"
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

const tunnelSessionTTL = 8 * time.Hour // upper bound; actual idle/session limits enforced by ExpiresAt-based context deadline

// ticketStreamLimiter throttles ticket-redemption ATTEMPTS per source IP on
// the public, unauthenticated /tunnels/stream endpoint. Tickets are 256-bit
// random and single-use, so guessing one is already cryptographically
// infeasible; this is defense-in-depth against brute-force/DoS traffic.
var ticketStreamLimiter = authpkg.NewRateLimiter(60, time.Minute)

type createTunnelRequest struct {
	TargetType string `json:"target_type"`
}

func (h *handlers) handleCreateTunnel(w http.ResponseWriter, r *http.Request) {
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

	var req createTunnelRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "target_type is required")
		return
	}

	var targetType remotesession.TargetType
	var permission string
	switch req.TargetType {
	case string(remotesession.TargetSSHLocal):
		targetType, permission = remotesession.TargetSSHLocal, authpkg.PermRemoteTunnelSSH
	case string(remotesession.TargetRDPLocal):
		targetType, permission = remotesession.TargetRDPLocal, authpkg.PermRemoteTunnelRDP
	default:
		writeErr(w, http.StatusBadRequest, "invalid_request", "target_type must be ssh_local or rdp_local")
		return
	}

	grants := authpkg.GrantsFromContext(r.Context())
	if !authpkg.HasPermission(grants, permission, deviceResource(d)) {
		writeErr(w, http.StatusForbidden, "permission_denied", "Not permitted")
		return
	}
	if !h.hub.IsOnline(d.ID) {
		writeErr(w, http.StatusConflict, "device_busy", "Device is not currently connected")
		return
	}

	user, _ := authpkg.UserFromContext(r.Context())
	sess, ticket, tunnelID, err := h.sessions.OpenTunnel(r.Context(), h.tunnels, d.ID, user.ID, targetType, tunnelSessionTTL, h.cfg.Relay.TicketTTL)
	if err != nil {
		code, msg, status := "internal_error", "Failed to open tunnel", http.StatusInternalServerError
		switch {
		case errors.Is(err, remotesession.ErrDeviceOffline):
			code, msg, status = "device_busy", "Device is not currently connected", http.StatusConflict
		case errors.Is(err, remotesession.ErrAgentRejected):
			// err wraps the agent's actual reason (e.g. "failed to reach
			// local target" when nothing is listening on 127.0.0.1:22/3389
			// on the device) — surface it instead of a generic message.
			code, msg, status = "agent_rejected", agentRejectionReason(err), http.StatusBadGateway
		}
		writeErr(w, status, code, msg)
		return
	}
	h.broker.Register(sess.ID, d.ID, protocol.StreamKindTunnel)

	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID, SessionID: &sess.ID,
		EventType: "tunnel.opened", Result: audit.ResultSuccess, SourceIP: clientIP(r),
		Metadata: map[string]any{"target_type": req.TargetType, "tunnel_id": tunnelID},
	})

	// helper_ticket is shown exactly once, per docs/API.md §9.
	writeJSON(w, http.StatusCreated, map[string]any{
		"tunnel_id":     tunnelID,
		"session_id":    sess.ID,
		"helper_ticket": ticket,
		"expires_at":    time.Now().UTC().Add(h.cfg.Relay.TicketTTL),
	}, nil)
}

// handleTunnelStream is the wr-helper-facing endpoint: authenticated purely
// by a single-use ticket (docs/RELAY.md §4), not a session cookie, since
// wr-helper is a native binary on the admin's machine with no browser
// session. It lives on the public listener alongside the agent gateway.
func (h *handlers) handleTunnelStream(w http.ResponseWriter, r *http.Request) {
	if !ticketStreamLimiter.Allow("tunnel-stream:" + clientIP(r)) {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "Too many ticket redemption attempts")
		return
	}
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "ticket is required")
		return
	}

	t, err := h.tunnels.ConsumeTicket(r.Context(), ticket)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "ticket_expired", "Ticket invalid, expired, or already used")
		return
	}

	sess, err := h.sessionRepo.Get(r.Context(), t.RemoteSessionID)
	if err != nil || sess.State != remotesession.StateActive {
		writeErr(w, http.StatusConflict, "session_expired", "Tunnel session is not active")
		return
	}
	stream, ok := h.broker.Get(sess.ID)
	if !ok {
		writeErr(w, http.StatusGone, "resource_not_found", "Stream no longer available")
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	deadlineCtx, cancelDeadline := context.WithDeadline(r.Context(), sess.ExpiresAt)
	defer cancelDeadline()
	// See the identical rationale in handlers_sessions.go handleSessionStream:
	// cancelling ctx as soon as the read loop exits is what unblocks the
	// write goroutine when wr-helper (not the agent) closes the connection
	// first — otherwise cleanup never runs.
	ctx, cancel := context.WithCancel(deadlineCtx)
	defer cancel()

	_ = h.audit.Record(ctx, audit.Event{
		ActorType: audit.ActorUser, ActorID: &t.UserID, DeviceID: &t.DeviceID, SessionID: &sess.ID,
		EventType: "tunnel.connected", Result: audit.ResultSuccess, SourceIP: clientIP(r),
		Metadata: map[string]any{"tunnel_id": t.ID},
	})

	var bytesDown, bytesUp int64

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
				bytesDown += int64(len(payload))
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
		if msgType == websocket.MessageBinary {
			_ = stream.Send(data)
			bytesUp += int64(len(data))
		}
	}
	cancel()

	<-done
	h.broker.Unregister(sess.ID)
	_ = h.tunnels.UpdateBytes(context.Background(), t.ID, bytesUp, bytesDown)
	_ = h.tunnels.Close(context.Background(), t.ID, "closed")
	if current, err := h.sessionRepo.Get(context.Background(), sess.ID); err == nil && current.State == remotesession.StateActive {
		_ = h.sessions.Close(context.Background(), sess, "helper_disconnected")
	}
	_ = h.audit.Record(context.Background(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &t.UserID, DeviceID: &t.DeviceID, SessionID: &sess.ID,
		EventType: "tunnel.closed", Result: audit.ResultSuccess, Metadata: map[string]any{"tunnel_id": t.ID, "reason": "helper_disconnected"},
	})
}
