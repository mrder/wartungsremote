package protocol

import "time"

// Parameter shapes for DeviceCommandPayload.Params, one per CommandType
// constant. Both server (constructing device_command) and agent (decoding
// Params) use these so the wire shape only needs to be defined once.

type ServiceActionParams struct {
	Name string `json:"name"`
}

type ProcessTerminateParams struct {
	PID             int32 `json:"pid"`
	StartTimeUnixMS int64 `json:"start_time_unix_ms"`
}

type FilesListParams struct {
	Path string `json:"path"`
}

type FilesMkdirParams struct {
	Path string `json:"path"`
}

type FilesRenameParams struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type FilesDeleteParams struct {
	Path string `json:"path"`
}

// FilesDownloadParams initiates an agent -> server file stream on StreamID
// (docs/PROTOCOL.md §11 binary stream frames, StreamKindFile). The agent
// signals end-of-file with one zero-length frame on the same stream.
type FilesDownloadParams struct {
	Path     string `json:"path"`
	StreamID string `json:"stream_id"`
}

// FilesUploadParams initiates a server -> agent file stream on StreamID.
// After the server sends a zero-length frame (EOF), the agent finalizes the
// write and replies with a command_result whose request_id is StreamID
// (not the original device_command message_id, since that request/response
// pair already completed with the initial "ready" acknowledgement).
type FilesUploadParams struct {
	Path     string `json:"path"`
	StreamID string `json:"stream_id"`
}

// LogsQueryParams requests a filtered log read (journalctl / Event Log).
type LogsQueryParams struct {
	Query string     `json:"query,omitempty"`
	Since *time.Time `json:"since,omitempty"`
	Until *time.Time `json:"until,omitempty"`
	Level string     `json:"level,omitempty"`
	Limit int        `json:"limit,omitempty"`
}

// AgentUpdateParams instructs the agent to self-update (docs/AGENT.md §15).
// ArtifactSHA256Hex and SignatureBase64 are re-verified by the agent
// against its own build-embedded trusted public key before anything is
// staged — the server having already checked the signature at release
// upload time is not sufficient, since a compromised server must not be
// able to push unsigned/tampered binaries to agents.
type AgentUpdateParams struct {
	Version           string `json:"version"`
	ArtifactURL       string `json:"artifact_url"`
	ArtifactSHA256Hex string `json:"artifact_sha256_hex"`
	SignatureBase64   string `json:"signature_base64"`
}
