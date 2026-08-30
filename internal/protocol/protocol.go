// Package protocol implements the WartungsRemote wire protocol (wrp/1)
// shared between wr-core and wr-agent, as defined in docs/PROTOCOL.md.
package protocol

import "time"

// Version is the currently implemented major protocol version.
const Version = 1

// Size limits per PROTOCOL.md §3.
const (
	MaxControlMessageBytes  = 256 * 1024
	MaxInventoryBytes       = 1024 * 1024
	MaxEventBatchBytes      = 1024 * 1024
	MaxTerminalControlBytes = 64 * 1024
)

// Message types.
const (
	TypeHello                   = "hello"
	TypeHelloAck                = "hello_ack"
	TypeHeartbeat               = "heartbeat"
	TypeHeartbeatAck            = "heartbeat_ack"
	TypeInventoryRequest        = "inventory_request"
	TypeInventoryResponse       = "inventory_response"
	TypeMetricsReport           = "metrics_report"
	TypeCommandResult           = "command_result"
	TypeSessionOpen             = "session_open"
	TypeSessionOpenResult       = "session_open_result"
	TypeSessionClose            = "session_close"
	TypeSessionPrivilegeUpdate  = "session_privilege_update"
	TypeTerminalOpen            = "terminal_open"
	TypeTerminalResize          = "terminal_resize"
	TypeTerminalSignal          = "terminal_signal"
	TypeTerminalClose           = "terminal_close"
	TypeTunnelPrepare           = "tunnel_prepare"
	TypeProtocolError           = "protocol_error"
	TypeControlChallenge        = "control_challenge"
	TypeDeviceCommand           = "device_command"
	TypeSupportCredentialReport = "support_credential_report"
	TypeNetworkMetricsBatch     = "network_metrics_batch"
)

// Binary stream frame kinds, per docs/PROTOCOL.md §11.
const (
	StreamKindTerminal byte = 1
	StreamKindTunnel   byte = 2
	StreamKindFile     byte = 3
)

// Error codes per PROTOCOL.md §9.
const (
	CodeOK                    = "ok"
	CodeInvalidRequest        = "invalid_request"
	CodeUnsupportedProtocol   = "unsupported_protocol"
	CodeUnsupportedCapability = "unsupported_capability"
	CodeUnauthenticated       = "unauthenticated"
	CodePermissionDenied      = "permission_denied"
	CodePrivilegeRequired     = "privilege_required"
	CodeDeviceBusy            = "device_busy"
	CodeResourceNotFound      = "resource_not_found"
	CodeSessionExpired        = "session_expired"
	CodeTicketExpired         = "ticket_expired"
	CodeTicketUsed            = "ticket_used"
	CodeTargetNotAllowed      = "target_not_allowed"
	CodeMessageTooLarge       = "message_too_large"
	CodeRateLimited           = "rate_limited"
	CodeTimeout               = "timeout"
	CodeInternalError         = "internal_error"
)

// Envelope is the mandatory outer structure for every control message.
type Envelope struct {
	Protocol  int        `json:"protocol"`
	Type      string     `json:"type"`
	MessageID string     `json:"message_id"`
	RequestID *string    `json:"request_id,omitempty"`
	Timestamp time.Time  `json:"timestamp"`
	Payload   RawPayload `json:"payload"`
}

// RawPayload defers payload decoding until the message Type is known.
type RawPayload = []byte

// ControlChallengePayload is sent by the server immediately after WebSocket
// upgrade, before any agent identity is known. The agent must sign Nonce
// with its Ed25519 device private key and echo it (base64) plus the
// signature in HelloPayload, proving possession of the private key for this
// specific connection attempt (replay-resistant: Nonce is single-use and
// short-lived). This binds control-channel authentication to the device
// identity established at enrollment without requiring mTLS infrastructure.
type ControlChallengePayload struct {
	Nonce     string    `json:"nonce"` // base64, 32 random bytes
	ExpiresAt time.Time `json:"expires_at"`
}

// HelloPayload is sent by the agent immediately after receiving ControlChallengePayload.
type HelloPayload struct {
	DeviceID     string   `json:"device_id"`
	InstallID    string   `json:"install_id"`
	AgentVersion string   `json:"agent_version"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	Capabilities []string `json:"capabilities"`
	BootID       string   `json:"boot_id"`
	Nonce        string   `json:"nonce"`     // echoes ControlChallengePayload.Nonce
	Signature    string   `json:"signature"` // base64 Ed25519 signature over the raw nonce bytes
	// Secure is true if the agent dialed the control channel via wss://
	// (i.e. its configured server_url is https://) — reported honestly by
	// the agent itself since wr-core, sitting behind whatever reverse
	// proxy terminates TLS, has no other way to know per-device whether
	// that specific agent is actually using an encrypted connection.
	Secure bool `json:"secure"`
}

// HelloAckPayload is the server's response to Hello.
type HelloAckPayload struct {
	ConnectionID             string    `json:"connection_id"`
	ServerTime               time.Time `json:"server_time"`
	HeartbeatIntervalSeconds int       `json:"heartbeat_interval_seconds"`
	StatusIntervalSeconds    int       `json:"status_interval_seconds"`
	MaxMessageBytes          int       `json:"max_message_bytes"`
	MinimumAgentVersion      string    `json:"minimum_agent_version"`
	// NetworkUploadIntervalSeconds controls how often the agent flushes its
	// locally-buffered network samples (see protocol.NetworkMetricsBatchPayload)
	// — server-controlled like StatusIntervalSeconds because it's what
	// governs how much control-channel traffic this generates, unlike the
	// local sampling cadence itself which is a purely agent-local concern.
	// An older agent that doesn't understand network_metrics_batch simply
	// never sends one; this field is harmless for it to receive.
	NetworkUploadIntervalSeconds int `json:"network_upload_interval_seconds"`
}

// HeartbeatPayload is sent periodically by the agent.
type HeartbeatPayload struct {
	UptimeSeconds int64 `json:"uptime_seconds"`
	Sequence      int64 `json:"sequence"`
}

// InventoryRequestPayload requests a (possibly partial) inventory refresh.
type InventoryRequestPayload struct {
	Full bool `json:"full"`
}

// OSInfo describes the operating system of a device.
type OSInfo struct {
	Family       string `json:"family"`
	Distribution string `json:"distribution,omitempty"`
	Version      string `json:"version"`
	Kernel       string `json:"kernel,omitempty"`
}

// CPUInfo describes the CPU of a device.
type CPUInfo struct {
	Model   string `json:"model"`
	Cores   int    `json:"cores"`
	Threads int    `json:"threads"`
}

// DiskInfo describes a mounted filesystem.
type DiskInfo struct {
	Path       string `json:"path"`
	Filesystem string `json:"filesystem,omitempty"`
	UsedBytes  uint64 `json:"used_bytes"`
	TotalBytes uint64 `json:"total_bytes"`
	// Removable is true for removable/USB/optical/network media, false for
	// fixed internal disks. Best-effort per platform; defaults to false
	// (treated as fixed) when the OS can't tell us. Health thresholds only
	// apply to non-removable disks — a full USB stick isn't a device-health
	// emergency the way a full system disk is.
	Removable bool `json:"removable"`
}

// InterfaceInfo describes a network interface.
type InterfaceInfo struct {
	Name       string   `json:"name"`
	MACAddress string   `json:"mac_address,omitempty"`
	IPv4       []string `json:"ipv4,omitempty"`
	IPv6       []string `json:"ipv6,omitempty"`
}

// InventoryResponsePayload carries the full system inventory.
type InventoryResponsePayload struct {
	Hostname      string          `json:"hostname"`
	OS            OSInfo          `json:"os"`
	CPU           CPUInfo         `json:"cpu"`
	MemoryBytes   uint64          `json:"memory_bytes"`
	Disks         []DiskInfo      `json:"disks"`
	Interfaces    []InterfaceInfo `json:"interfaces"`
	AgentVersion  string          `json:"agent_version"`
	BootTime      *time.Time      `json:"boot_time,omitempty"`
	UptimeSeconds int64           `json:"uptime_seconds"`
}

// MemoryUsage describes point-in-time memory usage.
type MemoryUsage struct {
	UsedBytes  uint64 `json:"used_bytes"`
	TotalBytes uint64 `json:"total_bytes"`
}

// FilesystemUsage describes point-in-time disk usage for one mount.
type FilesystemUsage struct {
	Path       string `json:"path"`
	UsedBytes  uint64 `json:"used_bytes"`
	TotalBytes uint64 `json:"total_bytes"`
	// Removable — see DiskInfo.Removable; health thresholds skip these.
	Removable bool `json:"removable"`
}

// MetricsReportPayload carries point-in-time live metrics.
type MetricsReportPayload struct {
	CPUPercent    float64           `json:"cpu_percent"`
	Memory        MemoryUsage       `json:"memory"`
	Filesystems   []FilesystemUsage `json:"filesystems"`
	UptimeSeconds int64             `json:"uptime_seconds"`
}

// SupportCredentialReportPayload is sent once by the agent (over the
// already-authenticated control channel — never a separate unauthenticated
// call) after it provisions or rotates the local remote-support OS account
// (docs/AGENT.md "Remote-support account"). The server encrypts Password
// at rest and only ever decrypts it on an explicit, audited dashboard
// reveal — see internal/support.
type SupportCredentialReportPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// NetworkMetricsSample is one buffered local observation, covering the
// period IntervalSeconds ending at OccurredAt. BytesSentTotal/RecvTotal
// are system-wide (all interfaces); BytesSentControl/RecvControl are just
// this agent's own control-channel traffic to this server — see
// docs/AGENT.md "Netzwerk-Traffic-Metriken" for why both are tracked
// separately (general bandwidth use vs. this tool's own overhead).
type NetworkMetricsSample struct {
	OccurredAt       time.Time `json:"occurred_at"`
	IntervalSeconds  float64   `json:"interval_seconds"`
	BytesSentTotal   uint64    `json:"bytes_sent_total"`
	BytesRecvTotal   uint64    `json:"bytes_recv_total"`
	BytesSentControl uint64    `json:"bytes_sent_control"`
	BytesRecvControl uint64    `json:"bytes_recv_control"`
}

// NetworkMetricsBatchPayload carries a batch of locally-buffered network
// samples, sent every NetworkUploadIntervalSeconds (and once immediately
// after connecting, to flush anything buffered while offline). The agent
// deletes its local copy of each sample only after this message has been
// successfully written to the control channel — see internal/netmetrics.
type NetworkMetricsBatchPayload struct {
	Samples []NetworkMetricsSample `json:"samples"`
}

// CommandResultPayload is a generic success/error response envelope.
type CommandResultPayload struct {
	Status  string `json:"status"` // "success" | "error"
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// ProtocolErrorPayload is sent when a message is rejected outright.
type ProtocolErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}
