// Package platform provides the OS-abstraction interfaces described in
// docs/AGENT.md §9, with concrete implementations under platform/linux and
// platform/windows selected at build time via the files in this package
// suffixed _linux.go / _windows.go (Go build tags), so cmd/wr-agent only
// ever calls platform.New().
package platform

import (
	"context"
	"io"
	"time"

	"wartungsremote/internal/protocol"
)

// InventoryProvider collects the one-time/on-change system inventory
// described in docs/PROTOCOL.md §6.
type InventoryProvider interface {
	Inventory(ctx context.Context) (protocol.InventoryResponsePayload, error)
}

// MetricsProvider collects point-in-time live metrics described in
// docs/PROTOCOL.md §7.
type MetricsProvider interface {
	Metrics(ctx context.Context) (protocol.MetricsReportPayload, error)
}

// TerminalSession is a single interactive shell process bound to one remote
// session, per docs/AGENT.md §10-11. Read returns PTY/ConPTY output; Write
// sends keyboard input. The session is bound to a single remote session and
// its process (group) is terminated on Close.
type TerminalSession interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// TerminalProvider opens interactive shell sessions.
type TerminalProvider interface {
	OpenTerminal(ctx context.Context) (TerminalSession, error)
}

// ServiceInfo describes one OS service/unit for docs/AGENT.md §14 /
// docs/API.md §11.
type ServiceInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Status      string `json:"status"` // "running" | "stopped" | "unknown"
}

// ServiceProvider manages OS services/units using native APIs — never shell
// string concatenation (docs/AGENT.md §14).
type ServiceProvider interface {
	ListServices(ctx context.Context) ([]ServiceInfo, error)
	StartService(ctx context.Context, name string) error
	StopService(ctx context.Context, name string) error
	RestartService(ctx context.Context, name string) error
}

// ProcessInfo describes one running process for docs/AGENT.md §14 /
// docs/API.md §12. StartTime is used together with PID to reduce PID-reuse
// risk when terminating.
type ProcessInfo struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryRSS  uint64  `json:"memory_rss_bytes"`
	Username   string  `json:"username,omitempty"`
	StartTime  int64   `json:"start_time_unix_ms"`
}

// ProcessProvider lists and terminates OS processes.
type ProcessProvider interface {
	ListProcesses(ctx context.Context) ([]ProcessInfo, error)
	TerminateProcess(ctx context.Context, pid int32, startTimeUnixMS int64) error
}

// FileEntry describes one directory entry for docs/AGENT.md §13 /
// docs/API.md §10.
type FileEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time_unix_ms"`
}

// FileProvider implements file operations with canonical path validation
// against traversal/symlink escape (docs/SECURITY.md §13).
type FileProvider interface {
	ListDir(ctx context.Context, path string) ([]FileEntry, error)
	Mkdir(ctx context.Context, path string) error
	Rename(ctx context.Context, from, to string) error
	Delete(ctx context.Context, path string) error
	ReadFile(ctx context.Context, path string) (io.ReadCloser, int64, error)
	WriteFile(ctx context.Context, path string) (io.WriteCloser, error)
}

// LogEntry is one normalized log line for docs/AGENT.md §25 / docs/API.md
// (journalctl on Linux, Windows Event Log on Windows).
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"` // "error" | "warning" | "info" | "unknown"
	Source  string    `json:"source"`
	Message string    `json:"message"`
}

// LogQuery narrows a log fetch. Zero values mean "no filter" for that field.
type LogQuery struct {
	Query string // free-text search
	Since *time.Time
	Until *time.Time
	Level string // "", "error", "warning", "info"
	Limit int
}

// LogProvider reads local system logs — never writes to them.
type LogProvider interface {
	QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, error)
}

// NetworkCounterProvider reads cumulative (since-boot) system-wide
// network byte counters, summed across all interfaces — the raw input
// internal/netmetrics turns into per-interval traffic samples.
type NetworkCounterProvider interface {
	NetworkCounters(ctx context.Context) (bytesSent, bytesRecv uint64, err error)
}

// SupportAccountProvider creates (or resets the password of) the dedicated
// local OS account used to log into the SSH/RDP tunnel this device
// exposes — a separate login from our own Ed25519 device identity, needed
// because the tunnel only forwards raw network traffic to the device's own
// existing SSH/RDP service (docs/AGENT.md "Remote-support account"). The
// generated password is returned once and never re-derivable — the server
// is the only place it's subsequently stored (encrypted).
type SupportAccountProvider interface {
	EnsureSupportAccount(ctx context.Context) (username, password string, err error)
}

// Provider bundles all capabilities implemented for the current OS.
type Provider interface {
	InventoryProvider
	MetricsProvider
	TerminalProvider
	ServiceProvider
	ProcessProvider
	FileProvider
	LogProvider
	SupportAccountProvider
	NetworkCounterProvider
	// Capabilities lists the capability strings this platform build
	// actually supports, reported in the control channel hello handshake.
	Capabilities() []string
}
