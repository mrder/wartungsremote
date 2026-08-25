package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
	"wartungsremote/internal/protocol"
)

const commandTimeout = 15 * time.Second

// runCommand sends a device_command and decodes its CommandResultPayload,
// mapping agent-side failure into the standard error envelope.
func (h *handlers) runCommand(w http.ResponseWriter, r *http.Request, deviceID uuid.UUID, cmdType string, params any) (protocol.CommandResultPayload, bool) {
	paramsJSON, merr := json.Marshal(params)
	if merr != nil {
		writeErr(w, http.StatusInternalServerError, "internal_error", "Failed to encode command")
		return protocol.CommandResultPayload{}, false
	}

	env, serr := h.hub.SendAndWait(r.Context(), deviceID, protocol.TypeDeviceCommand, protocol.DeviceCommandPayload{
		CommandType: cmdType,
		Params:      paramsJSON,
	}, commandTimeout)
	if serr != nil {
		writeErr(w, http.StatusConflict, "device_busy", "Device is not connected or did not respond in time")
		return protocol.CommandResultPayload{}, false
	}

	var result protocol.CommandResultPayload
	if err := protocol.DecodePayload(env, &result); err != nil {
		writeErr(w, http.StatusBadGateway, "internal_error", "Malformed agent response")
		return protocol.CommandResultPayload{}, false
	}
	if result.Status != "success" {
		status := http.StatusBadGateway
		if result.Code == protocol.CodePermissionDenied {
			status = http.StatusForbidden
		}
		writeErr(w, status, result.Code, result.Message)
		return protocol.CommandResultPayload{}, false
	}
	return result, true
}

// --- Services ---------------------------------------------------------------

func (h *handlers) handleListServices(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermDeviceRead)
	if !ok {
		return
	}
	result, ok := h.runCommand(w, r, d.ID, protocol.CmdServicesList, nil)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, result.Data, nil)
}

func (h *handlers) handleServiceAction(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteServiceControl)
	if !ok {
		return
	}
	serviceName := r.PathValue("service")
	action := lastPathSegment(r.URL.Path) // "start" | "stop" | "restart"

	var cmdType string
	switch action {
	case "start":
		cmdType = protocol.CmdServiceStart
	case "stop":
		cmdType = protocol.CmdServiceStop
	case "restart":
		cmdType = protocol.CmdServiceRestart
	default:
		writeErr(w, http.StatusBadRequest, "invalid_request", "unknown service action")
		return
	}

	if _, ok := h.runCommand(w, r, d.ID, cmdType, protocol.ServiceActionParams{Name: serviceName}); !ok {
		return
	}

	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID,
		EventType: "service." + action, Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"service": serviceName},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func lastPathSegment(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

// --- Processes ----------------------------------------------------------

func (h *handlers) handleListProcesses(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermDeviceRead)
	if !ok {
		return
	}
	result, ok := h.runCommand(w, r, d.ID, protocol.CmdProcessesList, nil)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, result.Data, nil)
}

// --- Logs -----------------------------------------------------------------

func (h *handlers) handleQueryLogs(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteFilesRead)
	if !ok {
		return
	}
	q := r.URL.Query()
	params := protocol.LogsQueryParams{Query: q.Get("query"), Level: q.Get("level")}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			params.Since = &t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			params.Until = &t
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			params.Limit = n
		}
	}

	result, ok := h.runCommand(w, r, d.ID, protocol.CmdLogsQuery, params)
	if !ok {
		return
	}

	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID,
		EventType: "logs.queried", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
	})
	writeJSON(w, http.StatusOK, result.Data, nil)
}

func (h *handlers) handleTerminateProcess(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteProcessTerminate)
	if !ok {
		return
	}
	pid, err := strconv.ParseInt(r.PathValue("pid"), 10, 32)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_request", "invalid pid")
		return
	}
	var body struct {
		StartTimeUnixMS int64 `json:"start_time_unix_ms"`
	}
	_ = decodeJSON(r, &body)

	if _, ok := h.runCommand(w, r, d.ID, protocol.CmdProcessTerminate, protocol.ProcessTerminateParams{
		PID: int32(pid), StartTimeUnixMS: body.StartTimeUnixMS,
	}); !ok {
		return
	}

	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &d.ID,
		EventType: "process.terminate", Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"pid": pid},
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}
