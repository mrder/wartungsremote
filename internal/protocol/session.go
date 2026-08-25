package protocol

import "time"

// SessionOpenPayload is sent Server -> Agent to request a new remote
// session, per docs/PROTOCOL.md §10.
type SessionOpenPayload struct {
	SessionID  string         `json:"session_id"`
	Kind       string         `json:"kind"` // "terminal"
	ExpiresAt  time.Time      `json:"expires_at"`
	Privileged bool           `json:"privileged"`
	Options    map[string]any `json:"options,omitempty"`
}

// SessionOpenResultPayload is the agent's Agent -> Server reply.
type SessionOpenResultPayload struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"` // "success" | "error"
	Code      string `json:"code"`
	Message   string `json:"message,omitempty"`
}

// SessionPrivilegeUpdatePayload notifies the agent that a session's
// privilege state changed, per docs/PROTOCOL.md §12. The agent MUST only
// accept this from the authenticated control connection for a session it
// has locally, per the same section.
type SessionPrivilegeUpdatePayload struct {
	SessionID       string    `json:"session_id"`
	Privileged      bool      `json:"privileged"`
	ValidUntil      time.Time `json:"valid_until,omitempty"`
	AuthorizationID string    `json:"authorization_id,omitempty"`
}

// SessionClosePayload closes a session from either side.
type SessionClosePayload struct {
	SessionID string `json:"session_id"`
	Reason    string `json:"reason,omitempty"`
}

// TerminalResizePayload carries a PTY window size change.
type TerminalResizePayload struct {
	SessionID string `json:"session_id"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// TerminalSignalPayload requests a signal be delivered to the terminal
// process group (Linux) or process tree (Windows).
type TerminalSignalPayload struct {
	SessionID string `json:"session_id"`
	Signal    string `json:"signal"`
}

// DeviceCommandPayload carries a single semantically-typed remote command
// (docs/SECURITY.md §11: "Keine generische exec arbitrary server command
// API"). CommandType selects a fixed, allowlisted operation; Params is
// validated by the agent against that specific operation's expected shape.
type DeviceCommandPayload struct {
	CommandID   string          `json:"command_id"`
	CommandType string          `json:"command_type"`
	Params      RawPayload      `json:"params"`
}

// Known device_command CommandType values.
const (
	CmdServicesList    = "services.list"
	CmdServiceStart    = "services.start"
	CmdServiceStop     = "services.stop"
	CmdServiceRestart  = "services.restart"
	CmdProcessesList   = "processes.list"
	CmdProcessTerminate = "processes.terminate"
	CmdFilesList       = "files.list"
	CmdFilesMkdir      = "files.mkdir"
	CmdFilesRename     = "files.rename"
	CmdFilesDelete     = "files.delete"
	CmdFilesDownload   = "files.download"
	CmdFilesUpload     = "files.upload"
	CmdLogsQuery       = "logs.query"
	CmdAgentUpdate     = "agent.update"
)
