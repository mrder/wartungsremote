// Package controlhub implements the authenticated agent control channel
// (WebSocket) per docs/PROTOCOL.md and docs/STATE_MACHINES.md §1.
package controlhub

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"wartungsremote/internal/audit"
	"wartungsremote/internal/device"
	"wartungsremote/internal/monitoring"
	"wartungsremote/internal/netutil"
	"wartungsremote/internal/protocol"
)

const minAgentVersion = "0.1.0"

type Timing struct {
	HeartbeatInterval     time.Duration
	ConnectionLostAfter   time.Duration
	OfflineAfter          time.Duration
	StatusInterval        time.Duration
	NetworkUploadInterval time.Duration
}

// BinaryFrameHandler receives inbound stream frames (terminal/tunnel/file
// bulk data) forwarded from an agent. Registered once by internal/relay.
type BinaryFrameHandler func(deviceID uuid.UUID, kind byte, streamID uuid.UUID, payload []byte)

// SessionCloseHandler receives agent-initiated session_close notifications,
// e.g. when a terminal process exits on its own (docs/STATE_MACHINES.md §3).
// Registered once by the remote session wiring in internal/httpapi.
type SessionCloseHandler func(deviceID uuid.UUID, sessionID, reason string)

// DeviceDisconnectHandler is invoked once the agent's control-channel
// connection ends, so any active remote sessions for that device can be
// interrupted immediately rather than left dangling until their own
// expiry (docs/RELAY.md §8: "Bei Agent Disconnect: alle zugehörigen
// Streams sofort schließen").
type DeviceDisconnectHandler func(deviceID uuid.UUID)

// VersionBlockedChecker reports whether an agent self-reporting this exact
// (osFamily, architecture, version) at handshake should be refused
// (docs/SECURITY.md §20 "Agent-Version als blockiert markieren"). Kept as
// a callback rather than a direct import of internal/agentrelease to avoid
// controlhub depending on that package, matching the rest of Hub's
// pluggable-handler pattern.
type VersionBlockedChecker func(ctx context.Context, osFamily, architecture, version string) (bool, error)

// SupportCredentialHandler persists the dedicated local remote-support OS
// account credential an agent reports after provisioning or rotating it.
// Kept as a callback rather than a direct import of internal/support, same
// reasoning as VersionBlockedChecker.
type SupportCredentialHandler func(ctx context.Context, deviceID uuid.UUID, username, password string) error

type Hub struct {
	devices        *device.Repo
	health         *monitoring.Engine
	audit          *audit.Logger
	timing         Timing
	trustedProxies netutil.TrustedProxies

	mu    sync.Mutex
	conns map[uuid.UUID]*connection // keyed by device ID

	binaryHandlerMu sync.RWMutex
	binaryHandler   BinaryFrameHandler

	sessionCloseHandlerMu sync.RWMutex
	sessionCloseHandler   SessionCloseHandler

	disconnectHandlerMu sync.RWMutex
	disconnectHandler   DeviceDisconnectHandler

	versionBlockedMu sync.RWMutex
	versionBlocked   VersionBlockedChecker

	supportCredentialHandlerMu sync.RWMutex
	supportCredentialHandler   SupportCredentialHandler
}

func NewHub(devices *device.Repo, health *monitoring.Engine, auditLogger *audit.Logger, timing Timing, trustedProxies netutil.TrustedProxies) *Hub {
	return &Hub{
		devices:        devices,
		health:         health,
		audit:          auditLogger,
		timing:         timing,
		trustedProxies: trustedProxies,
		conns:          make(map[uuid.UUID]*connection),
	}
}

// SetBinaryFrameHandler registers the callback used to route inbound binary
// stream frames. Must be called once during startup wiring.
func (h *Hub) SetBinaryFrameHandler(fn BinaryFrameHandler) {
	h.binaryHandlerMu.Lock()
	h.binaryHandler = fn
	h.binaryHandlerMu.Unlock()
}

// SetSessionCloseHandler registers the callback used to react to
// agent-initiated session_close notifications. Must be called once during
// startup wiring.
func (h *Hub) SetSessionCloseHandler(fn SessionCloseHandler) {
	h.sessionCloseHandlerMu.Lock()
	h.sessionCloseHandler = fn
	h.sessionCloseHandlerMu.Unlock()
}

// SetDeviceDisconnectHandler registers the callback invoked when an agent's
// control connection ends. Must be called once during startup wiring.
func (h *Hub) SetDeviceDisconnectHandler(fn DeviceDisconnectHandler) {
	h.disconnectHandlerMu.Lock()
	h.disconnectHandler = fn
	h.disconnectHandlerMu.Unlock()
}

// SetVersionBlockedChecker registers the callback consulted at every
// handshake to refuse connections from an explicitly blocked agent
// version. Must be called once during startup wiring.
func (h *Hub) SetVersionBlockedChecker(fn VersionBlockedChecker) {
	h.versionBlockedMu.Lock()
	h.versionBlocked = fn
	h.versionBlockedMu.Unlock()
}

// SetSupportCredentialHandler registers the callback that persists a
// reported remote-support account credential. Must be called once during
// startup wiring.
func (h *Hub) SetSupportCredentialHandler(fn SupportCredentialHandler) {
	h.supportCredentialHandlerMu.Lock()
	h.supportCredentialHandler = fn
	h.supportCredentialHandlerMu.Unlock()
}

type connection struct {
	deviceID  uuid.UUID
	installID uuid.UUID
	remoteIP  string
	conn      *websocket.Conn
	cancel    context.CancelFunc

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan protocol.Envelope
}

func (c *connection) write(ctx context.Context, msgType websocket.MessageType, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.Write(ctx, msgType, data)
}

// sendAndWait sends a new message and blocks until a response referencing
// it by request_id arrives, or ctx/timeout elapses. Used for session_open
// and device_command, where the caller needs the agent's outcome.
func (c *connection) sendAndWait(ctx context.Context, msgType string, payload any, timeout time.Duration) (protocol.Envelope, error) {
	raw, msgID, err := encodeWithID(msgType, payload)
	if err != nil {
		return protocol.Envelope{}, err
	}

	ch := make(chan protocol.Envelope, 1)
	c.pendingMu.Lock()
	if c.pending == nil {
		c.pending = make(map[string]chan protocol.Envelope)
	}
	c.pending[msgID] = ch
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, msgID)
		c.pendingMu.Unlock()
	}()

	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.write(writeCtx, websocket.MessageText, raw); err != nil {
		return protocol.Envelope{}, fmt.Errorf("controlhub: send: %w", err)
	}

	waitCtx, cancel2 := context.WithTimeout(ctx, timeout)
	defer cancel2()
	select {
	case env := <-ch:
		return env, nil
	case <-waitCtx.Done():
		return protocol.Envelope{}, fmt.Errorf("controlhub: %w waiting for agent response", waitCtx.Err())
	}
}

// waitForID registers a pending waiter under an arbitrary correlation ID
// (not necessarily a message this connection sent) and blocks for a
// matching request_id, without sending anything itself. Used when the
// initiating request already got its own immediate reply and a second,
// later, asynchronous completion signal is expected under a different,
// pre-agreed ID (e.g. file upload finalization keyed by stream ID).
func (c *connection) waitForID(ctx context.Context, id string, timeout time.Duration) (protocol.Envelope, error) {
	ch := make(chan protocol.Envelope, 1)
	c.pendingMu.Lock()
	if c.pending == nil {
		c.pending = make(map[string]chan protocol.Envelope)
	}
	c.pending[id] = ch
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case env := <-ch:
		return env, nil
	case <-waitCtx.Done():
		return protocol.Envelope{}, fmt.Errorf("controlhub: %w waiting for correlated response", waitCtx.Err())
	}
}

// tryDeliver hands env to a pending sendAndWait caller if its request_id
// matches one. Returns true if it was consumed this way (no further
// processing needed).
func (c *connection) tryDeliver(env protocol.Envelope) bool {
	if env.RequestID == nil {
		return false
	}
	c.pendingMu.Lock()
	ch, ok := c.pending[*env.RequestID]
	if ok {
		delete(c.pending, *env.RequestID)
	}
	c.pendingMu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- env:
	default:
	}
	return true
}

func encodeWithID(msgType string, payload any) (raw []byte, messageID string, err error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("controlhub: marshal payload: %w", err)
	}
	env := protocol.Envelope{
		Protocol:  protocol.Version,
		Type:      msgType,
		MessageID: protocol.NewMessageID(),
		Timestamp: time.Now().UTC(),
		Payload:   body,
	}
	raw, err = json.Marshal(env)
	if err != nil {
		return nil, "", fmt.Errorf("controlhub: marshal envelope: %w", err)
	}
	return raw, env.MessageID, nil
}

// Run starts the background sweeper that enforces the time-based
// connectivity state machine (ONLINE -> CONNECTION_LOST -> OFFLINE) even for
// agents that vanish without a clean close.
func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.sweep(ctx)
		}
	}
}

func (h *Hub) sweep(ctx context.Context) {
	h.mu.Lock()
	ids := make([]uuid.UUID, 0, len(h.conns))
	for id := range h.conns {
		ids = append(ids, id)
	}
	h.mu.Unlock()

	for _, id := range ids {
		d, err := h.devices.GetByID(ctx, id)
		if err != nil || d.LastSeenAt == nil {
			continue
		}
		since := time.Since(*d.LastSeenAt)
		switch {
		case since >= h.timing.OfflineAfter && d.Status != device.StatusOffline:
			_ = h.devices.UpdateStatusOnly(ctx, id, device.StatusOffline)
			h.closeConn(id, "offline_timeout")
			_, _, _ = h.health.Evaluate(ctx, id)
		case since >= h.timing.ConnectionLostAfter && d.Status == device.StatusOnline:
			_ = h.devices.UpdateStatusOnly(ctx, id, device.StatusConnectionLost)
		}
	}
}

func (h *Hub) closeConn(id uuid.UUID, reason string) {
	h.mu.Lock()
	c, ok := h.conns[id]
	if ok {
		delete(h.conns, id)
	}
	h.mu.Unlock()
	if ok {
		c.cancel()
		_ = c.conn.Close(websocket.StatusPolicyViolation, reason)
	}
}

// IsOnline reports whether a device currently has a live control connection.
func (h *Hub) IsOnline(id uuid.UUID) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.conns[id]
	return ok
}

func (h *Hub) getConn(id uuid.UUID) (*connection, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c, ok := h.conns[id]
	return c, ok
}

// RequestInventory asks a connected agent for a fresh full inventory
// snapshot, used by POST /devices/:id/status-request.
func (h *Hub) RequestInventory(ctx context.Context, id uuid.UUID) error {
	c, ok := h.getConn(id)
	if !ok {
		return fmt.Errorf("controlhub: device not connected")
	}
	raw, err := protocol.Encode(protocol.TypeInventoryRequest, nil, protocol.InventoryRequestPayload{Full: true})
	if err != nil {
		return err
	}
	return c.write(ctx, websocket.MessageText, raw)
}

// SendAndWait sends msgType/payload to the connected agent for deviceID and
// blocks for a correlated response (by request_id), or returns an error on
// timeout/disconnect. Used for session_open and device_command.
func (h *Hub) SendAndWait(ctx context.Context, deviceID uuid.UUID, msgType string, payload any, timeout time.Duration) (protocol.Envelope, error) {
	c, ok := h.getConn(deviceID)
	if !ok {
		return protocol.Envelope{}, fmt.Errorf("controlhub: device not connected")
	}
	return c.sendAndWait(ctx, msgType, payload, timeout)
}

// WaitForCorrelation blocks until the connected agent for deviceID sends a
// message whose request_id equals correlationID. See connection.waitForID.
func (h *Hub) WaitForCorrelation(ctx context.Context, deviceID uuid.UUID, correlationID string, timeout time.Duration) (protocol.Envelope, error) {
	c, ok := h.getConn(deviceID)
	if !ok {
		return protocol.Envelope{}, fmt.Errorf("controlhub: device not connected")
	}
	return c.waitForID(ctx, correlationID, timeout)
}

// SendMessage sends msgType/payload to the connected agent without waiting
// for a response (e.g. terminal_resize, session_close).
func (h *Hub) SendMessage(ctx context.Context, deviceID uuid.UUID, msgType string, payload any) error {
	c, ok := h.getConn(deviceID)
	if !ok {
		return fmt.Errorf("controlhub: device not connected")
	}
	raw, err := protocol.Encode(msgType, nil, payload)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.write(writeCtx, websocket.MessageText, raw)
}

// SendBinaryFrame forwards a stream frame (terminal/tunnel/file bulk data)
// to the connected agent for deviceID.
func (h *Hub) SendBinaryFrame(deviceID uuid.UUID, kind byte, streamID uuid.UUID, payload []byte) error {
	c, ok := h.getConn(deviceID)
	if !ok {
		return fmt.Errorf("controlhub: device not connected")
	}
	frame := protocol.EncodeStreamFrame(kind, streamID, payload)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.write(ctx, websocket.MessageBinary, frame)
}

// ServeAgentWS upgrades the connection, performs the challenge/response
// device authentication handshake, then runs the read loop until the
// connection ends.
func (h *Hub) ServeAgentWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Subprotocol negotiation not required for V1; same-origin/CORS is
		// enforced upstream by only exposing this on the public gateway
		// listener for agents, not the admin browser origin.
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(int64(protocol.MaxInventoryBytes))

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	deviceID, installID, ok := h.handshake(ctx, conn)
	if !ok {
		_ = conn.Close(websocket.StatusPolicyViolation, "authentication failed")
		return
	}

	c := &connection{deviceID: deviceID, installID: installID, remoteIP: h.clientIP(r), conn: conn, cancel: cancel, pending: make(map[string]chan protocol.Envelope)}
	h.mu.Lock()
	if old, exists := h.conns[deviceID]; exists {
		// Only one active primary control session per install ID: close the
		// previous one in a controlled way (docs/SPECIFICATION.md §9).
		delete(h.conns, deviceID)
		old.cancel()
		go old.conn.Close(websocket.StatusPolicyViolation, "superseded_by_new_connection")
	}
	h.conns[deviceID] = c
	h.mu.Unlock()

	_ = h.devices.UpdateConnectivity(ctx, deviceID, device.StatusOnline, h.clientIP(r))
	_, _, _ = h.health.Evaluate(ctx, deviceID)
	_ = h.audit.Record(ctx, audit.Event{
		ActorType: audit.ActorAgent,
		DeviceID:  &deviceID,
		EventType: audit.EventDeviceConnected,
		Result:    audit.ResultSuccess,
		SourceIP:  h.clientIP(r),
	})
	slog.Info("agent connected", "device_id", deviceID, "install_id", installID)

	h.readLoop(ctx, c)

	h.mu.Lock()
	if h.conns[deviceID] == c {
		delete(h.conns, deviceID)
	}
	h.mu.Unlock()
	_ = h.devices.UpdateStatusOnly(context.Background(), deviceID, device.StatusConnectionLost)
	_ = h.audit.Record(context.Background(), audit.Event{
		ActorType: audit.ActorAgent,
		DeviceID:  &deviceID,
		EventType: audit.EventDeviceDisconnected,
		Result:    audit.ResultSuccess,
	})
	h.disconnectHandlerMu.RLock()
	handler := h.disconnectHandler
	h.disconnectHandlerMu.RUnlock()
	if handler != nil {
		handler(deviceID)
	}
	slog.Info("agent disconnected", "device_id", deviceID)
}

// handshake sends a control_challenge, waits for a matching signed hello,
// verifies the Ed25519 signature against the device's stored public key,
// and returns the authenticated device/install IDs.
func (h *Hub) handshake(ctx context.Context, conn *websocket.Conn) (deviceID, installID uuid.UUID, ok bool) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	expiresAt := time.Now().UTC().Add(30 * time.Second)
	challengeRaw, err := protocol.Encode(protocol.TypeControlChallenge, nil, protocol.ControlChallengePayload{
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}

	hsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := conn.Write(hsCtx, websocket.MessageText, challengeRaw); err != nil {
		return uuid.Nil, uuid.Nil, false
	}

	_, raw, err := conn.Read(hsCtx)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	env, err := protocol.Decode(raw)
	if err != nil || env.Type != protocol.TypeHello || env.Protocol != protocol.Version {
		return uuid.Nil, uuid.Nil, false
	}
	var hello protocol.HelloPayload
	if err := protocol.DecodePayload(env, &hello); err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	if hello.Nonce != base64.StdEncoding.EncodeToString(nonce) {
		return uuid.Nil, uuid.Nil, false
	}

	devID, err := uuid.Parse(hello.DeviceID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	instID, err := uuid.Parse(hello.InstallID)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}

	d, err := h.devices.GetByID(hsCtx, devID)
	if err != nil || d.InstallID != instID || d.Status == device.StatusRevoked || d.CredentialStatus != "active" {
		return uuid.Nil, uuid.Nil, false
	}

	h.versionBlockedMu.RLock()
	checker := h.versionBlocked
	h.versionBlockedMu.RUnlock()
	if checker != nil {
		if blocked, err := checker(hsCtx, osFamilyOf(hello.OS), hello.Arch, hello.AgentVersion); err == nil && blocked {
			slog.Warn("rejected handshake from blocked agent version", "device_id", devID, "version", hello.AgentVersion)
			return uuid.Nil, uuid.Nil, false
		}
	}

	pubKey, err := h.devices.ActiveCredentialPublicKey(hsCtx, devID)
	if err != nil || len(pubKey) != ed25519.PublicKeySize {
		return uuid.Nil, uuid.Nil, false
	}
	sig, err := base64.StdEncoding.DecodeString(hello.Signature)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	if !ed25519.Verify(ed25519.PublicKey(pubKey), nonce, sig) {
		return uuid.Nil, uuid.Nil, false
	}

	ackRaw, err := protocol.Encode(protocol.TypeHelloAck, &env.MessageID, protocol.HelloAckPayload{
		ConnectionID:                 uuid.NewString(),
		ServerTime:                   time.Now().UTC(),
		HeartbeatIntervalSeconds:     int(h.timing.HeartbeatInterval.Seconds()),
		StatusIntervalSeconds:        int(h.timing.StatusInterval.Seconds()),
		MaxMessageBytes:              protocol.MaxInventoryBytes,
		MinimumAgentVersion:          minAgentVersion,
		NetworkUploadIntervalSeconds: int(h.timing.NetworkUploadInterval.Seconds()),
	})
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	if err := conn.Write(hsCtx, websocket.MessageText, ackRaw); err != nil {
		return uuid.Nil, uuid.Nil, false
	}

	if len(hello.Capabilities) > 0 {
		_ = h.devices.SetCapabilities(context.Background(), devID, hello.Capabilities)
	}
	_ = h.devices.ApplyInventory(context.Background(), devID, d.Hostname, osFamilyOf(hello.OS), d.OSName, d.OSVersion, hello.Arch, hello.AgentVersion)

	return devID, instID, true
}

func osFamilyOf(os string) string {
	if os == "" {
		return "unknown"
	}
	return os
}

// clientIP returns a bare IP suitable for a Postgres `inet` column; see the
// identical rationale in internal/httpapi.clientIP.
func (h *Hub) clientIP(r *http.Request) string {
	return netutil.ClientIP(r, h.trustedProxies)
}

func (h *Hub) readLoop(ctx context.Context, c *connection) {
	for {
		msgType, raw, err := c.conn.Read(ctx)
		if err != nil {
			return
		}

		if msgType == websocket.MessageBinary {
			h.handleBinaryFrame(c, raw)
			continue
		}

		if len(raw) > protocol.MaxInventoryBytes {
			h.protocolError(ctx, c, protocol.CodeMessageTooLarge, "message exceeds maximum size")
			continue
		}
		env, err := protocol.Decode(raw)
		if err != nil {
			h.protocolError(ctx, c, protocol.CodeInvalidRequest, "malformed envelope")
			continue
		}
		if c.tryDeliver(env) {
			continue
		}
		h.handleMessage(ctx, c, env)
	}
}

func (h *Hub) handleBinaryFrame(c *connection, raw []byte) {
	kind, streamID, payload, err := protocol.DecodeStreamFrame(raw)
	if err != nil {
		return
	}
	h.binaryHandlerMu.RLock()
	handler := h.binaryHandler
	h.binaryHandlerMu.RUnlock()
	if handler != nil {
		handler(c.deviceID, kind, streamID, payload)
	}
}

func (h *Hub) handleMessage(ctx context.Context, c *connection, env protocol.Envelope) {
	switch env.Type {
	case protocol.TypeHeartbeat:
		var hb protocol.HeartbeatPayload
		if err := protocol.DecodePayload(env, &hb); err != nil {
			h.protocolError(ctx, c, protocol.CodeInvalidRequest, "malformed heartbeat")
			return
		}
		_ = h.devices.UpdateConnectivity(ctx, c.deviceID, device.StatusOnline, c.remoteIP)

	case protocol.TypeInventoryResponse:
		if len(env.Payload) > protocol.MaxInventoryBytes {
			h.protocolError(ctx, c, protocol.CodeMessageTooLarge, "inventory too large")
			return
		}
		var inv protocol.InventoryResponsePayload
		if err := protocol.DecodePayload(env, &inv); err != nil {
			h.protocolError(ctx, c, protocol.CodeInvalidRequest, "malformed inventory")
			return
		}
		_ = h.devices.ApplyInventory(ctx, c.deviceID, inv.Hostname, inv.OS.Family, inv.OS.Distribution, inv.OS.Version, "", inv.AgentVersion)
		_ = h.devices.RecordNetwork(ctx, c.deviceID, inv.Interfaces, c.remoteIP)
		_, _, _ = h.health.Evaluate(ctx, c.deviceID)

	case protocol.TypeMetricsReport:
		if len(env.Payload) > protocol.MaxEventBatchBytes {
			h.protocolError(ctx, c, protocol.CodeMessageTooLarge, "metrics report too large")
			return
		}
		var m protocol.MetricsReportPayload
		if err := protocol.DecodePayload(env, &m); err != nil {
			h.protocolError(ctx, c, protocol.CodeInvalidRequest, "malformed metrics report")
			return
		}
		_ = h.devices.RecordMetrics(ctx, c.deviceID, m.CPUPercent, m.Memory.UsedBytes, m.Memory.TotalBytes, m.Filesystems, m.UptimeSeconds)
		_, _, _ = h.health.Evaluate(ctx, c.deviceID)

	case protocol.TypeNetworkMetricsBatch:
		if len(env.Payload) > protocol.MaxEventBatchBytes {
			h.protocolError(ctx, c, protocol.CodeMessageTooLarge, "network metrics batch too large")
			return
		}
		var nb protocol.NetworkMetricsBatchPayload
		if err := protocol.DecodePayload(env, &nb); err != nil {
			h.protocolError(ctx, c, protocol.CodeInvalidRequest, "malformed network metrics batch")
			return
		}
		if err := h.devices.RecordNetworkMetricsBatch(ctx, c.deviceID, nb.Samples); err != nil {
			slog.Error("failed to record network metrics batch", "device_id", c.deviceID, "error", err)
		}

	case protocol.TypeSupportCredentialReport:
		var sc protocol.SupportCredentialReportPayload
		if err := protocol.DecodePayload(env, &sc); err != nil {
			h.protocolError(ctx, c, protocol.CodeInvalidRequest, "malformed support credential report")
			return
		}
		h.supportCredentialHandlerMu.RLock()
		handler := h.supportCredentialHandler
		h.supportCredentialHandlerMu.RUnlock()
		if handler != nil {
			if err := handler(ctx, c.deviceID, sc.Username, sc.Password); err != nil {
				slog.Error("failed to store support credential", "device_id", c.deviceID, "error", err)
			}
		}

	case protocol.TypeCommandResult, protocol.TypeSessionOpenResult:
		// A response whose request_id didn't match any pending waiter
		// (already timed out, or unsolicited) — nothing to do but avoid
		// treating it as an unsupported/protocol-error message type.

	case protocol.TypeSessionClose:
		// Agent-initiated close notification (e.g. the terminal process
		// exited on its own), distinct from the server-initiated
		// session_close sent via SendMessage. Forward it so the owning
		// remote session gets cleaned up server-side too.
		var sc protocol.SessionClosePayload
		if err := protocol.DecodePayload(env, &sc); err != nil {
			return
		}
		h.sessionCloseHandlerMu.RLock()
		handler := h.sessionCloseHandler
		h.sessionCloseHandlerMu.RUnlock()
		if handler != nil {
			handler(c.deviceID, sc.SessionID, sc.Reason)
		}

	default:
		h.protocolError(ctx, c, protocol.CodeInvalidRequest, fmt.Sprintf("unsupported message type %q", env.Type))
		_ = h.audit.Record(ctx, audit.Event{
			ActorType: audit.ActorAgent,
			DeviceID:  &c.deviceID,
			EventType: audit.EventProtocolError,
			Result:    audit.ResultFailure,
			Metadata:  map[string]any{"type": env.Type},
		})
	}
}

func (h *Hub) protocolError(ctx context.Context, c *connection, code, message string) {
	raw, err := protocol.Encode(protocol.TypeProtocolError, nil, protocol.ProtocolErrorPayload{Code: code, Message: message})
	if err != nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = c.write(writeCtx, websocket.MessageText, raw)
}
