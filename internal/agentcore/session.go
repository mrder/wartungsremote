package agentcore

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"wartungsremote/internal/config"
	"wartungsremote/internal/netmetrics"
	"wartungsremote/internal/platform"
	"wartungsremote/internal/protocol"
)

// networkBatchSize caps how many buffered samples go into one
// network_metrics_batch message, keeping it comfortably under
// protocol.MaxEventBatchBytes even for a device that's been offline a
// while; uploadNetworkMetrics sends as many batches as needed to drain
// everything pending on each tick.
const networkBatchSize = 500

// agentSession bundles everything a single control-channel connection needs
// to serve heartbeats, metrics, and remote-session/command dispatch
// concurrently (heartbeat ticker, terminal I/O pumps, and the read loop all
// write to the same *websocket.Conn, hence writeMu).
type agentSession struct {
	conn     *websocket.Conn
	writeMu  sync.Mutex
	provider platform.Provider
	policy   config.AgentPolicy
	sessions *sessionManager
	files    *fileTransferManager
	dataDir  string
	netStore *netmetrics.Store
	netBytes *netmetrics.ControlBytesCounter
}

func (a *agentSession) write(ctx context.Context, msgType websocket.MessageType, data []byte) error {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	err := a.conn.Write(ctx, msgType, data)
	if err == nil && a.netBytes != nil {
		// Message payload bytes, not raw wire bytes — see
		// netmetrics.ControlBytesCounter's doc comment for why that's an
		// accepted approximation here.
		a.netBytes.AddSent(len(data))
	}
	return err
}

func (a *agentSession) writeEnvelope(ctx context.Context, msgType string, requestID *string, payload any) error {
	raw, err := protocol.Encode(msgType, requestID, payload)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return a.write(writeCtx, websocket.MessageText, raw)
}

func (a *agentSession) writeBinaryFrame(kind byte, streamID uuid.UUID, payload []byte) error {
	frame := protocol.EncodeStreamFrame(kind, streamID, payload)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return a.write(ctx, websocket.MessageBinary, frame)
}

// runSession performs a single control-channel connection attempt: dial,
// challenge/response handshake, then heartbeat + inventory/metrics loop
// until the connection ends or ctx is cancelled. It returns nil only when
// ctx is cancelled (graceful shutdown); any connection failure returns an
// error so the caller's reconnect loop applies backoff.
func runSession(ctx context.Context, serverURL, agentVersion string, identity Identity, provider platform.Provider, policy config.AgentPolicy, dataDir string, netStore *netmetrics.Store, netBytes *netmetrics.ControlBytesCounter, onConnected func()) error {
	wsURL := strings.Replace(strings.TrimRight(serverURL, "/"), "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/v1/agent/control"

	dialCtx, cancelDial := context.WithTimeout(ctx, 30*time.Second)
	defer cancelDial()
	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("agentcore: dial control channel: %w", err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(int64(protocol.MaxInventoryBytes))

	secure := strings.HasPrefix(wsURL, "wss://")
	heartbeatInterval, statusInterval, networkUploadInterval, err := handshake(ctx, conn, agentVersion, identity, provider, secure)
	if err != nil {
		return err
	}

	as := &agentSession{conn: conn, provider: provider, policy: policy, sessions: newSessionManager(), files: newFileTransferManager(), dataDir: dataDir, netStore: netStore, netBytes: netBytes}
	defer as.sessions.closeAll()
	defer as.files.closeAll()

	slog.Info("control channel established", "device_id", identity.DeviceID, "heartbeat_interval", heartbeatInterval)
	onConnected()
	if policy.SSHTunnel || policy.RDPTunnel {
		go as.ensureSupportAccountProvisioned(ctx, provider)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- as.readLoop(ctx) }()

	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()
	statusTicker := time.NewTicker(statusInterval)
	defer statusTicker.Stop()
	networkUploadTicker := time.NewTicker(networkUploadInterval)
	defer networkUploadTicker.Stop()

	var seq int64
	as.sendMetrics(ctx)
	// Flush anything buffered locally while disconnected, immediately on
	// (re)connect rather than waiting a full upload interval.
	as.uploadNetworkMetrics(ctx)

	for {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "shutdown")
			return nil
		case err := <-errCh:
			return err
		case <-heartbeatTicker.C:
			seq++
			if err := as.writeEnvelope(ctx, protocol.TypeHeartbeat, nil, protocol.HeartbeatPayload{
				UptimeSeconds: uptimeSeconds(ctx, provider),
				Sequence:      seq,
			}); err != nil {
				return fmt.Errorf("agentcore: send heartbeat: %w", err)
			}
		case <-statusTicker.C:
			as.sendMetrics(ctx)
		case <-networkUploadTicker.C:
			as.uploadNetworkMetrics(ctx)
		}
	}
}

// uploadNetworkMetrics drains everything currently buffered in netStore,
// sending it as one or more network_metrics_batch messages, deleting each
// batch locally only after it's been successfully written to the control
// channel. Best-effort: a failure just leaves the rows for the next tick
// (or the next connection's immediate flush) to retry, same reliability
// bar as every other report on this channel (no ack tracking).
func (a *agentSession) uploadNetworkMetrics(ctx context.Context) {
	if a.netStore == nil {
		return
	}
	for {
		pending, err := a.netStore.Pending(ctx, networkBatchSize)
		if err != nil {
			slog.Warn("failed to read pending network samples", "error", err)
			return
		}
		if len(pending) == 0 {
			return
		}

		samples := make([]protocol.NetworkMetricsSample, len(pending))
		for i, s := range pending {
			samples[i] = protocol.NetworkMetricsSample{
				OccurredAt:       s.OccurredAt,
				IntervalSeconds:  s.IntervalSeconds,
				BytesSentTotal:   s.BytesSentTotal,
				BytesRecvTotal:   s.BytesRecvTotal,
				BytesSentControl: s.BytesSentControl,
				BytesRecvControl: s.BytesRecvControl,
			}
		}
		if err := a.writeEnvelope(ctx, protocol.TypeNetworkMetricsBatch, nil, protocol.NetworkMetricsBatchPayload{Samples: samples}); err != nil {
			slog.Warn("failed to upload network metrics batch", "error", err)
			return
		}
		if err := a.netStore.DeleteUpTo(ctx, pending[len(pending)-1].ID); err != nil {
			slog.Warn("failed to clear uploaded network samples", "error", err)
			return
		}
		if len(pending) < networkBatchSize {
			return
		}
	}
}

// ensureSupportAccountProvisioned creates the remote-support OS account and
// reports its credential exactly once (tracked by a local marker file, not
// per-connection state, so a reconnect never re-triggers it) — see
// docs/AGENT.md "Remote-support account". A failure here is retried on the
// next reconnect: the marker is only written after the report is
// successfully sent, and EnsureSupportAccount itself is idempotent (a
// no-op re-creation, just a fresh password) if the account already exists.
func (a *agentSession) ensureSupportAccountProvisioned(ctx context.Context, provider platform.Provider) {
	marker := filepath.Join(a.dataDir, "support_account_provisioned")
	if _, err := os.Stat(marker); err == nil {
		return
	}
	username, password, err := provider.EnsureSupportAccount(ctx)
	if err != nil {
		slog.Error("failed to provision remote-support account", "error", err)
		return
	}
	if err := a.writeEnvelope(ctx, protocol.TypeSupportCredentialReport, nil, protocol.SupportCredentialReportPayload{
		Username: username, Password: password,
	}); err != nil {
		slog.Error("failed to report remote-support credential", "error", err)
		return
	}
	if err := os.WriteFile(marker, []byte("1"), 0o600); err != nil {
		slog.Warn("remote-support account provisioned but could not write marker file; will re-provision (harmless — resets its password again) on next reconnect", "error", err)
	}
}

func uptimeSeconds(ctx context.Context, provider platform.Provider) int64 {
	inv, err := provider.Inventory(ctx)
	if err != nil {
		return 0
	}
	return inv.UptimeSeconds
}

func (a *agentSession) sendMetrics(ctx context.Context) {
	m, err := a.provider.Metrics(ctx)
	if err != nil {
		slog.Warn("collect metrics failed", "error", err)
		return
	}
	if err := a.writeEnvelope(ctx, protocol.TypeMetricsReport, nil, m); err != nil {
		slog.Warn("send metrics failed", "error", err)
	}
}

// handshake completes the control_challenge/hello/hello_ack exchange
// described in docs/PROTOCOL.md §4 and the Ed25519 proof-of-possession
// scheme implemented server-side in internal/controlhub.
func handshake(ctx context.Context, conn *websocket.Conn, agentVersion string, identity Identity, provider platform.Provider, secure bool) (heartbeatInterval, statusInterval, networkUploadInterval time.Duration, err error) {
	hsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, raw, err := conn.Read(hsCtx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("agentcore: read challenge: %w", err)
	}
	env, err := protocol.Decode(raw)
	if err != nil || env.Type != protocol.TypeControlChallenge {
		return 0, 0, 0, fmt.Errorf("agentcore: unexpected message instead of control_challenge")
	}
	var challenge protocol.ControlChallengePayload
	if err := protocol.DecodePayload(env, &challenge); err != nil {
		return 0, 0, 0, fmt.Errorf("agentcore: decode challenge: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(challenge.Nonce)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("agentcore: decode nonce: %w", err)
	}

	signature := identity.Sign(nonce)

	helloRaw, err := protocol.Encode(protocol.TypeHello, nil, protocol.HelloPayload{
		DeviceID:     identity.DeviceID.String(),
		InstallID:    identity.InstallID.String(),
		AgentVersion: agentVersion,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Capabilities: provider.Capabilities(),
		BootID:       uuid.NewString(),
		Nonce:        challenge.Nonce,
		Signature:    base64.StdEncoding.EncodeToString(signature),
		Secure:       secure,
	})
	if err != nil {
		return 0, 0, 0, err
	}
	if err := conn.Write(hsCtx, websocket.MessageText, helloRaw); err != nil {
		return 0, 0, 0, fmt.Errorf("agentcore: send hello: %w", err)
	}

	_, ackRaw, err := conn.Read(hsCtx)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("agentcore: read hello_ack: %w", err)
	}
	ackEnv, err := protocol.Decode(ackRaw)
	if err != nil || ackEnv.Type != protocol.TypeHelloAck {
		return 0, 0, 0, fmt.Errorf("agentcore: authentication rejected by server")
	}
	var ack protocol.HelloAckPayload
	if err := protocol.DecodePayload(ackEnv, &ack); err != nil {
		return 0, 0, 0, fmt.Errorf("agentcore: decode hello_ack: %w", err)
	}

	hb := time.Duration(ack.HeartbeatIntervalSeconds) * time.Second
	if hb <= 0 {
		hb = 45 * time.Second
	}
	status := time.Duration(ack.StatusIntervalSeconds) * time.Second
	if status <= 0 {
		status = 5 * time.Minute
	}
	networkUpload := time.Duration(ack.NetworkUploadIntervalSeconds) * time.Second
	if networkUpload <= 0 {
		networkUpload = 5 * time.Minute
	}
	return hb, status, networkUpload, nil
}

func (a *agentSession) readLoop(ctx context.Context) error {
	for {
		msgType, raw, err := a.conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("agentcore: connection closed: %w", err)
		}
		if a.netBytes != nil {
			a.netBytes.AddRecv(len(raw))
		}

		if msgType == websocket.MessageBinary {
			a.handleBinaryFrame(raw)
			continue
		}

		env, err := protocol.Decode(raw)
		if err != nil {
			continue
		}
		a.handleMessage(ctx, env)
	}
}

func (a *agentSession) handleMessage(ctx context.Context, env protocol.Envelope) {
	switch env.Type {
	case protocol.TypeInventoryRequest:
		inv, err := a.provider.Inventory(ctx)
		if err != nil {
			slog.Warn("collect inventory failed", "error", err)
			return
		}
		_ = a.writeEnvelope(ctx, protocol.TypeInventoryResponse, &env.MessageID, inv)

	case protocol.TypeSessionOpen:
		a.handleSessionOpen(ctx, env)
	case protocol.TypeTerminalResize:
		a.handleTerminalResize(env)
	case protocol.TypeTerminalSignal:
		// V1: no cross-platform signal delivery implemented beyond close.
	case protocol.TypeSessionClose, protocol.TypeTerminalClose:
		a.handleSessionClose(env)
	case protocol.TypeSessionPrivilegeUpdate:
		// Recorded for future privilege-gated actions; V1 terminal doesn't
		// yet vary local behavior by privilege state.
	case protocol.TypeDeviceCommand:
		a.handleDeviceCommand(ctx, env)

	case protocol.TypeProtocolError:
		var pe protocol.ProtocolErrorPayload
		if err := protocol.DecodePayload(env, &pe); err == nil {
			slog.Warn("server reported protocol error", "code", pe.Code, "message", pe.Message)
		}
	}
}
