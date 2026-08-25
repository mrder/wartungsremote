package agentcore

import (
	"context"
	"io"
	"sync"

	"github.com/google/uuid"

	"wartungsremote/internal/protocol"
)

// fileTransferManager tracks in-progress uploads (server -> agent writes)
// for one control-channel connection, keyed by stream ID. Downloads don't
// need shared state: each is a single goroutine reading a file and pushing
// frames until EOF.
type fileTransferManager struct {
	mu      sync.Mutex
	uploads map[uuid.UUID]io.WriteCloser
}

func newFileTransferManager() *fileTransferManager {
	return &fileTransferManager{uploads: make(map[uuid.UUID]io.WriteCloser)}
}

func (m *fileTransferManager) addUpload(id uuid.UUID, w io.WriteCloser) {
	m.mu.Lock()
	m.uploads[id] = w
	m.mu.Unlock()
}

func (m *fileTransferManager) getUpload(id uuid.UUID) (io.WriteCloser, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.uploads[id]
	return w, ok
}

func (m *fileTransferManager) removeUpload(id uuid.UUID) (io.WriteCloser, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.uploads[id]
	if ok {
		delete(m.uploads, id)
	}
	return w, ok
}

func (m *fileTransferManager) closeAll() {
	m.mu.Lock()
	writers := make([]io.WriteCloser, 0, len(m.uploads))
	for id, w := range m.uploads {
		writers = append(writers, w)
		delete(m.uploads, id)
	}
	m.mu.Unlock()
	for _, w := range writers {
		_ = w.Close()
	}
}

// pumpFileDownload streams a file's contents to the server as binary stream
// frames, terminated by one zero-length frame, per docs/PROTOCOL.md §11.
func (a *agentSession) pumpFileDownload(streamID uuid.UUID, rc io.ReadCloser) {
	defer rc.Close()
	buf := make([]byte, 64*1024)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			if werr := a.writeBinaryFrame(protocol.StreamKindFile, streamID, buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			break
		}
	}
	_ = a.writeBinaryFrame(protocol.StreamKindFile, streamID, nil) // EOF marker
}

// handleFileBinaryFrame routes an inbound StreamKindFile frame to the
// matching upload writer; a zero-length payload finalizes and closes it,
// then replies with a command_result correlated by stream ID (see
// protocol.FilesUploadParams).
func (a *agentSession) handleFileBinaryFrame(streamID uuid.UUID, payload []byte) {
	w, ok := a.files.getUpload(streamID)
	if !ok {
		return
	}
	if len(payload) == 0 {
		a.files.removeUpload(streamID)
		err := w.Close()
		streamIDStr := streamID.String()
		status, code, msg := "success", protocol.CodeOK, ""
		if err != nil {
			status, code, msg = "error", protocol.CodeInternalError, err.Error()
		}
		_ = a.writeEnvelope(context.Background(), protocol.TypeCommandResult, &streamIDStr, protocol.CommandResultPayload{
			Status: status, Code: code, Message: msg,
		})
		return
	}
	if _, err := w.Write(payload); err != nil {
		a.files.removeUpload(streamID)
		_ = w.Close()
	}
}
