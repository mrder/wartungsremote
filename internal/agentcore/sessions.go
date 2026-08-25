package agentcore

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"

	"wartungsremote/internal/platform"
	"wartungsremote/internal/protocol"
)

// sessionManager tracks active remote sessions (terminal or tunnel) for one
// control-channel connection, each a plain io.ReadWriteCloser: a
// PTY/ConPTY for "terminal", or a raw loopback TCP connection for
// "ssh_local"/"rdp_local" (docs/AGENT.md §12).
type sessionManager struct {
	mu      sync.Mutex
	streams map[uuid.UUID]io.ReadWriteCloser
}

func newSessionManager() *sessionManager {
	return &sessionManager{streams: make(map[uuid.UUID]io.ReadWriteCloser)}
}

func (m *sessionManager) add(id uuid.UUID, s io.ReadWriteCloser) {
	m.mu.Lock()
	m.streams[id] = s
	m.mu.Unlock()
}

func (m *sessionManager) get(id uuid.UUID) (io.ReadWriteCloser, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.streams[id]
	return s, ok
}

func (m *sessionManager) remove(id uuid.UUID) (io.ReadWriteCloser, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.streams[id]
	if ok {
		delete(m.streams, id)
	}
	return s, ok
}

func (m *sessionManager) closeAll() {
	m.mu.Lock()
	sessions := make([]io.ReadWriteCloser, 0, len(m.streams))
	for id, s := range m.streams {
		sessions = append(sessions, s)
		delete(m.streams, id)
	}
	m.mu.Unlock()
	for _, s := range sessions {
		_ = s.Close()
	}
}

// handleSessionOpen implements docs/PROTOCOL.md §10-11: the agent MUST
// independently verify capability and local policy before honoring a
// session request — the server permission check alone is not sufficient
// (docs/SECURITY.md §11).
func (a *agentSession) handleSessionOpen(ctx context.Context, env protocol.Envelope) {
	var req protocol.SessionOpenPayload
	if err := protocol.DecodePayload(env, &req); err != nil {
		a.replySessionOpenResult(ctx, env.MessageID, "", "error", protocol.CodeInvalidRequest, "malformed session_open")
		return
	}

	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		a.replySessionOpenResult(ctx, env.MessageID, req.SessionID, "error", protocol.CodeInvalidRequest, "invalid session_id")
		return
	}

	switch req.Kind {
	case "terminal":
		a.openTerminalSession(ctx, env.MessageID, sessionID)
	case "ssh_local":
		a.openTunnelSession(ctx, env.MessageID, sessionID, "127.0.0.1:22", a.policy.SSHTunnel)
	case "rdp_local":
		a.openTunnelSession(ctx, env.MessageID, sessionID, "127.0.0.1:3389", a.policy.RDPTunnel)
	default:
		a.replySessionOpenResult(ctx, env.MessageID, req.SessionID, "error", protocol.CodeUnsupportedCapability, fmt.Sprintf("unsupported session kind %q", req.Kind))
	}
}

func (a *agentSession) openTerminalSession(ctx context.Context, requestID string, sessionID uuid.UUID) {
	if !a.policy.Terminal {
		a.replySessionOpenResult(ctx, requestID, sessionID.String(), "error", protocol.CodePermissionDenied, "terminal disabled by local agent policy")
		return
	}

	term, err := a.provider.OpenTerminal(ctx)
	if err != nil {
		slog.Warn("open terminal failed", "error", err)
		a.replySessionOpenResult(ctx, requestID, sessionID.String(), "error", protocol.CodeInternalError, "failed to open terminal")
		return
	}
	a.sessions.add(sessionID, term)

	go a.pumpStream(sessionID, term, protocol.StreamKindTerminal)

	a.replySessionOpenResult(ctx, requestID, sessionID.String(), "success", protocol.CodeOK, "")
}

// openTunnelSession connects only to the fixed loopback target implied by
// the semantic kind the server asked for — never to a host/port taken from
// the request payload itself (docs/SECURITY.md §12, docs/AGENT.md §12: "Kein
// Aufbau, der ungeprüft host/port aus Remoteinput übernimmt").
func (a *agentSession) openTunnelSession(ctx context.Context, requestID string, sessionID uuid.UUID, target string, allowed bool) {
	if !allowed {
		a.replySessionOpenResult(ctx, requestID, sessionID.String(), "error", protocol.CodePermissionDenied, "tunnel disabled by local agent policy")
		return
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", target)
	if err != nil {
		slog.Warn("open tunnel target failed", "target", target, "error", err)
		a.replySessionOpenResult(ctx, requestID, sessionID.String(), "error", protocol.CodeInternalError, "failed to reach local target")
		return
	}
	a.sessions.add(sessionID, conn)

	go a.pumpStream(sessionID, conn, protocol.StreamKindTunnel)

	a.replySessionOpenResult(ctx, requestID, sessionID.String(), "success", protocol.CodeOK, "")
}

// pumpStream streams a session's output (PTY/ConPTY or tunnel TCP socket)
// to the server as binary stream frames until the session is closed or the
// underlying stream ends.
func (a *agentSession) pumpStream(sessionID uuid.UUID, rwc io.ReadWriteCloser, kind byte) {
	buf := make([]byte, 32*1024)
	for {
		n, err := rwc.Read(buf)
		if n > 0 {
			if werr := a.writeBinaryFrame(kind, sessionID, buf[:n]); werr != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
	if _, ok := a.sessions.remove(sessionID); ok {
		_ = rwc.Close()
	}
	_ = a.writeEnvelope(context.Background(), protocol.TypeSessionClose, nil, protocol.SessionClosePayload{
		SessionID: sessionID.String(),
		Reason:    "stream_ended",
	})
}

func (a *agentSession) handleTerminalResize(env protocol.Envelope) {
	var req protocol.TerminalResizePayload
	if err := protocol.DecodePayload(env, &req); err != nil {
		return
	}
	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		return
	}
	// Only PTY/ConPTY sessions support resize; tunnels (raw TCP) don't
	// implement platform.TerminalSession, so the type assertion simply
	// fails silently for them.
	if s, ok := a.sessions.get(sessionID); ok {
		if term, ok := s.(platform.TerminalSession); ok {
			_ = term.Resize(req.Cols, req.Rows)
		}
	}
}

func (a *agentSession) handleSessionClose(env protocol.Envelope) {
	var req protocol.SessionClosePayload
	if err := protocol.DecodePayload(env, &req); err != nil {
		return
	}
	sessionID, err := uuid.Parse(req.SessionID)
	if err != nil {
		return
	}
	if s, ok := a.sessions.remove(sessionID); ok {
		_ = s.Close()
	}
}

func (a *agentSession) handleBinaryFrame(raw []byte) {
	kind, streamID, payload, err := protocol.DecodeStreamFrame(raw)
	if err != nil {
		return
	}
	switch kind {
	case protocol.StreamKindTerminal, protocol.StreamKindTunnel:
		if s, ok := a.sessions.get(streamID); ok {
			_, _ = s.Write(payload)
		}
	case protocol.StreamKindFile:
		a.handleFileBinaryFrame(streamID, payload)
	}
}

func (a *agentSession) replySessionOpenResult(ctx context.Context, requestID, sessionID, status, code, message string) {
	_ = a.writeEnvelope(ctx, protocol.TypeSessionOpenResult, &requestID, protocol.SessionOpenResultPayload{
		SessionID: sessionID,
		Status:    status,
		Code:      code,
		Message:   message,
	})
}
