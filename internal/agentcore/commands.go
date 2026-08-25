package agentcore

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"

	"wartungsremote/internal/platform"
	"wartungsremote/internal/protocol"
)

// handleDeviceCommand dispatches a single semantically-typed command
// (docs/SECURITY.md §11: no generic exec API) to the local platform
// provider, subject to local agent policy in addition to whatever
// server-side permission already gated sending it.
func (a *agentSession) handleDeviceCommand(ctx context.Context, env protocol.Envelope) {
	var cmd protocol.DeviceCommandPayload
	if err := protocol.DecodePayload(env, &cmd); err != nil {
		a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed device_command", nil)
		return
	}

	switch cmd.CommandType {
	case protocol.CmdServicesList:
		if !a.policy.ServiceControl {
			a.deny(ctx, env.MessageID, "service control disabled by local agent policy")
			return
		}
		list, err := a.provider.ListServices(ctx)
		a.replyResult(ctx, env.MessageID, err, list)

	case protocol.CmdServiceStart, protocol.CmdServiceStop, protocol.CmdServiceRestart:
		if !a.policy.ServiceControl {
			a.deny(ctx, env.MessageID, "service control disabled by local agent policy")
			return
		}
		var p protocol.ServiceActionParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		var err error
		switch cmd.CommandType {
		case protocol.CmdServiceStart:
			err = a.provider.StartService(ctx, p.Name)
		case protocol.CmdServiceStop:
			err = a.provider.StopService(ctx, p.Name)
		case protocol.CmdServiceRestart:
			err = a.provider.RestartService(ctx, p.Name)
		}
		a.replyResult(ctx, env.MessageID, err, nil)

	case protocol.CmdProcessesList:
		if !a.policy.ProcessTerminate {
			// Listing is allowed even if termination isn't, per
			// docs/SPECIFICATION.md §18 ("Lesen kann ... erhalten").
		}
		list, err := a.provider.ListProcesses(ctx)
		a.replyResult(ctx, env.MessageID, err, list)

	case protocol.CmdProcessTerminate:
		if !a.policy.ProcessTerminate {
			a.deny(ctx, env.MessageID, "process termination disabled by local agent policy")
			return
		}
		var p protocol.ProcessTerminateParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		err := a.provider.TerminateProcess(ctx, p.PID, p.StartTimeUnixMS)
		a.replyResult(ctx, env.MessageID, err, nil)

	case protocol.CmdFilesList:
		if !a.policy.FilesRead {
			a.deny(ctx, env.MessageID, "file access disabled by local agent policy")
			return
		}
		var p protocol.FilesListParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		list, err := a.provider.ListDir(ctx, p.Path)
		a.replyResult(ctx, env.MessageID, err, list)

	case protocol.CmdFilesMkdir:
		if !a.policy.FilesWrite {
			a.deny(ctx, env.MessageID, "file write disabled by local agent policy")
			return
		}
		var p protocol.FilesMkdirParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		err := a.provider.Mkdir(ctx, p.Path)
		a.replyResult(ctx, env.MessageID, err, nil)

	case protocol.CmdFilesRename:
		if !a.policy.FilesWrite {
			a.deny(ctx, env.MessageID, "file write disabled by local agent policy")
			return
		}
		var p protocol.FilesRenameParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		err := a.provider.Rename(ctx, p.From, p.To)
		a.replyResult(ctx, env.MessageID, err, nil)

	case protocol.CmdFilesDelete:
		if !a.policy.FilesWrite {
			a.deny(ctx, env.MessageID, "file write disabled by local agent policy")
			return
		}
		var p protocol.FilesDeleteParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		err := a.provider.Delete(ctx, p.Path)
		a.replyResult(ctx, env.MessageID, err, nil)

	case protocol.CmdLogsQuery:
		if !a.policy.FilesRead {
			// Log access reuses the files-read policy flag: it's a
			// read-only diagnostic capability with the same sensitivity
			// class as reading arbitrary files (docs/PROJECT_CONCEPT.md §25).
			a.deny(ctx, env.MessageID, "log access disabled by local agent policy")
			return
		}
		var p protocol.LogsQueryParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		entries, err := a.provider.QueryLogs(ctx, platform.LogQuery{
			Query: p.Query, Since: p.Since, Until: p.Until, Level: p.Level, Limit: p.Limit,
		})
		a.replyResult(ctx, env.MessageID, err, entries)

	case protocol.CmdFilesDownload:
		if !a.policy.FilesRead {
			a.deny(ctx, env.MessageID, "file access disabled by local agent policy")
			return
		}
		var p protocol.FilesDownloadParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		streamID, err := uuid.Parse(p.StreamID)
		if err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "invalid stream_id", nil)
			return
		}
		rc, size, err := a.provider.ReadFile(ctx, p.Path)
		if err != nil {
			a.replyResult(ctx, env.MessageID, err, nil)
			return
		}
		a.replyResult(ctx, env.MessageID, nil, map[string]any{"size": size})
		go a.pumpFileDownload(streamID, rc)

	case protocol.CmdAgentUpdate:
		if !a.policy.SelfUpdate {
			a.deny(ctx, env.MessageID, "self-update disabled by local agent policy")
			return
		}
		var p protocol.AgentUpdateParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		if err := a.beginUpdate(p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInternalError, err.Error(), nil)
			return
		}
		// Download+verify+stage already succeeded; the process exits
		// shortly so a service manager (or, in dev, an operator) relaunches
		// the now-updated binary. The reply is best-effort — the server
		// should not rely on it, since the process is about to die; the
		// agent's next `hello` with the new agent_version is the real
		// confirmation (docs/AGENT.md §15 step 10 "Health Signal").
		a.replyCommand(ctx, env.MessageID, "success", protocol.CodeOK, "update staged, restarting", nil)
		go func() {
			time.Sleep(2 * time.Second) // give the reply a chance to reach the server before the process exits
			os.Exit(0)
		}()

	case protocol.CmdFilesUpload:
		if !a.policy.FilesWrite {
			a.deny(ctx, env.MessageID, "file write disabled by local agent policy")
			return
		}
		var p protocol.FilesUploadParams
		if err := decodeParams(cmd.Params, &p); err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "malformed params", nil)
			return
		}
		streamID, err := uuid.Parse(p.StreamID)
		if err != nil {
			a.replyCommand(ctx, env.MessageID, "error", protocol.CodeInvalidRequest, "invalid stream_id", nil)
			return
		}
		wc, err := a.provider.WriteFile(ctx, p.Path)
		if err != nil {
			a.replyResult(ctx, env.MessageID, err, nil)
			return
		}
		a.files.addUpload(streamID, wc)
		a.replyResult(ctx, env.MessageID, nil, nil)

	default:
		a.replyCommand(ctx, env.MessageID, "error", protocol.CodeUnsupportedCapability, "unknown command_type", nil)
	}
}

func decodeParams(raw protocol.RawPayload, dst any) error {
	env := protocol.Envelope{Payload: raw}
	return protocol.DecodePayload(env, dst)
}

func (a *agentSession) deny(ctx context.Context, requestID, message string) {
	a.replyCommand(ctx, requestID, "error", protocol.CodePermissionDenied, message, nil)
}

func (a *agentSession) replyResult(ctx context.Context, requestID string, err error, data any) {
	if err != nil {
		a.replyCommand(ctx, requestID, "error", protocol.CodeInternalError, err.Error(), nil)
		return
	}
	a.replyCommand(ctx, requestID, "success", protocol.CodeOK, "", data)
}

func (a *agentSession) replyCommand(ctx context.Context, requestID, status, code, message string, data any) {
	_ = a.writeEnvelope(ctx, protocol.TypeCommandResult, &requestID, protocol.CommandResultPayload{
		Status:  status,
		Code:    code,
		Message: message,
		Data:    data,
	})
}
