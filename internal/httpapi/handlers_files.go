package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/google/uuid"

	"wartungsremote/internal/audit"
	authpkg "wartungsremote/internal/auth"
	"wartungsremote/internal/protocol"
)

// maxUploadBytes is the V1 hard cap on a single file upload, per
// docs/SPECIFICATION.md §17 ("max. Datei- und Transfergröße konfigurierbar").
// A future settings API can make this admin-configurable; the constant
// keeps the current, always-enforced ceiling explicit.
const maxUploadBytes = 500 * 1024 * 1024

func (h *handlers) handleFilesList(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteFilesRead)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "path is required")
		return
	}
	result, ok := h.runCommand(w, r, d.ID, protocol.CmdFilesList, protocol.FilesListParams{Path: p})
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, result.Data, nil)
}

func (h *handlers) handleFilesMkdir(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteFilesWrite)
	if !ok {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Path == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "path is required")
		return
	}
	if _, ok := h.runCommand(w, r, d.ID, protocol.CmdFilesMkdir, protocol.FilesMkdirParams{Path: req.Path}); !ok {
		return
	}
	h.auditFileOp(r, d.ID, "file.mkdir", req.Path)
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleFilesRename(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteFilesWrite)
	if !ok {
		return
	}
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeJSON(r, &req); err != nil || req.From == "" || req.To == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "from and to are required")
		return
	}
	if _, ok := h.runCommand(w, r, d.ID, protocol.CmdFilesRename, protocol.FilesRenameParams{From: req.From, To: req.To}); !ok {
		return
	}
	h.auditFileOp(r, d.ID, "file.rename", req.From+" -> "+req.To)
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) handleFilesDelete(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteFilesWrite)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "path is required")
		return
	}
	if _, ok := h.runCommand(w, r, d.ID, protocol.CmdFilesDelete, protocol.FilesDeleteParams{Path: p}); !ok {
		return
	}
	h.auditFileOp(r, d.ID, "file.delete", p)
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok"}, nil)
}

func (h *handlers) auditFileOp(r *http.Request, deviceID uuid.UUID, eventType, path string) {
	user, _ := authpkg.UserFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: audit.ActorUser, ActorID: &user.ID, DeviceID: &deviceID,
		EventType: eventType, Result: audit.ResultSuccess, SourceIP: h.clientIP(r),
		Metadata: map[string]any{"path": path},
	})
}

// handleFilesDownload streams a remote file to the browser. It never
// buffers the whole file in memory: bytes are forwarded to the HTTP
// response as they arrive from the agent over the relay broker.
func (h *handlers) handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteFilesRead)
	if !ok {
		return
	}
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "path is required")
		return
	}

	streamID := uuid.New()
	stream := h.broker.Register(streamID, d.ID, protocol.StreamKindFile)
	defer h.broker.Unregister(streamID)

	env, err := h.hub.SendAndWait(r.Context(), d.ID, protocol.TypeDeviceCommand, protocol.DeviceCommandPayload{
		CommandType: protocol.CmdFilesDownload,
		Params:      mustJSON(protocol.FilesDownloadParams{Path: filePath, StreamID: streamID.String()}),
	}, commandTimeout)
	if err != nil {
		writeErr(w, http.StatusConflict, "device_busy", "Device is not connected or did not respond in time")
		return
	}
	var result protocol.CommandResultPayload
	if err := protocol.DecodePayload(env, &result); err != nil || result.Status != "success" {
		writeErr(w, http.StatusNotFound, "resource_not_found", "File not found or not readable")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", path.Base(filePath)))
	if sizeVal, ok := asFloat(result.Data, "size"); ok {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", int64(sizeVal)))
	}
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	for {
		select {
		case chunk, more := <-stream.FromAgent():
			if !more || len(chunk) == 0 {
				h.auditFileOp(r, d.ID, "file.download", filePath)
				return
			}
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		case <-ctx.Done():
			return
		}
	}
}

// handleFilesUpload streams the request body to the agent as it arrives,
// finalizing with an atomic rename agent-side (docs/SPECIFICATION.md §17).
func (h *handlers) handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	d, ok := h.loadDeviceWithAccess(w, r, authpkg.PermRemoteFilesWrite)
	if !ok {
		return
	}
	targetPath := r.URL.Query().Get("path")
	if targetPath == "" {
		writeErr(w, http.StatusBadRequest, "invalid_request", "path is required")
		return
	}

	streamID := uuid.New()
	stream := h.broker.Register(streamID, d.ID, protocol.StreamKindFile)
	defer h.broker.Unregister(streamID)

	env, err := h.hub.SendAndWait(r.Context(), d.ID, protocol.TypeDeviceCommand, protocol.DeviceCommandPayload{
		CommandType: protocol.CmdFilesUpload,
		Params:      mustJSON(protocol.FilesUploadParams{Path: targetPath, StreamID: streamID.String()}),
	}, commandTimeout)
	if err != nil {
		writeErr(w, http.StatusConflict, "device_busy", "Device is not connected or did not respond in time")
		return
	}
	var ready protocol.CommandResultPayload
	if err := protocol.DecodePayload(env, &ready); err != nil || ready.Status != "success" {
		writeErr(w, http.StatusForbidden, "permission_denied", "Agent rejected the upload")
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxUploadBytes)
	buf := make([]byte, 64*1024)
	var total int64
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			total += int64(n)
			if err := stream.Send(buf[:n]); err != nil {
				writeErr(w, http.StatusBadGateway, "internal_error", "Upload interrupted")
				return
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			writeErr(w, http.StatusBadRequest, "invalid_request", "Upload too large or interrupted")
			return
		}
	}
	_ = stream.Send(nil) // EOF marker, see protocol.FilesUploadParams

	finalEnv, err := h.hub.WaitForCorrelation(r.Context(), d.ID, streamID.String(), 30*time.Second)
	if err != nil {
		writeErr(w, http.StatusGatewayTimeout, "timeout", "Agent did not confirm the upload")
		return
	}
	var final protocol.CommandResultPayload
	if err := protocol.DecodePayload(finalEnv, &final); err != nil || final.Status != "success" {
		writeErr(w, http.StatusBadGateway, "internal_error", "Agent failed to finalize the upload")
		return
	}

	h.auditFileOp(r, d.ID, "file.upload", fmt.Sprintf("%s (%d bytes)", targetPath, total))
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok", "bytes": total}, nil)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func asFloat(data any, key string) (float64, bool) {
	m, ok := data.(map[string]any)
	if !ok {
		return 0, false
	}
	v, ok := m[key].(float64)
	return v, ok
}
