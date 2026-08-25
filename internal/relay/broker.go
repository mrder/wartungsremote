// Package relay bridges binary stream frames (terminal I/O, and later
// file/tunnel bulk data) between the browser-facing admin WebSocket and the
// agent's control-channel binary frames, keyed by stream ID. This is the
// "Relay" role from docs/RELAY.md, implemented in-process as part of
// wr-core for V1 per docs/SPECIFICATION.md §2 (Gateway/Core/Relay may be
// combined into a single service for MVP).
package relay

import (
	"sync"

	"github.com/google/uuid"

	"wartungsremote/internal/controlhub"
)

type Stream struct {
	ID       uuid.UUID
	DeviceID uuid.UUID
	Kind     byte

	toBrowser chan []byte

	hub *controlhub.Hub
}

// Send forwards browser -> agent bytes for this stream.
func (s *Stream) Send(payload []byte) error {
	return s.hub.SendBinaryFrame(s.DeviceID, s.Kind, s.ID, payload)
}

// FromAgent is the channel of agent -> browser bytes to forward out over
// the browser WebSocket.
func (s *Stream) FromAgent() <-chan []byte {
	return s.toBrowser
}

type Broker struct {
	hub *controlhub.Hub

	mu      sync.Mutex
	streams map[uuid.UUID]*Stream
}

func NewBroker(hub *controlhub.Hub) *Broker {
	b := &Broker{hub: hub, streams: make(map[uuid.UUID]*Stream)}
	hub.SetBinaryFrameHandler(b.handleInbound)
	return b
}

// Register creates a new routable stream for streamID (typically the
// remote_session ID). Must be called before the agent starts sending
// frames for it (i.e. after session_open_result succeeds).
func (b *Broker) Register(streamID, deviceID uuid.UUID, kind byte) *Stream {
	s := &Stream{
		ID:        streamID,
		DeviceID:  deviceID,
		Kind:      kind,
		toBrowser: make(chan []byte, 64),
		hub:       b.hub,
	}
	b.mu.Lock()
	b.streams[streamID] = s
	b.mu.Unlock()
	return s
}

// Get returns the stream registered for streamID, if any.
func (b *Broker) Get(streamID uuid.UUID) (*Stream, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.streams[streamID]
	return s, ok
}

// Unregister removes a stream and stops routing frames to it. Safe to call
// even if the stream was never registered or already removed.
func (b *Broker) Unregister(streamID uuid.UUID) {
	b.mu.Lock()
	s, ok := b.streams[streamID]
	if ok {
		delete(b.streams, streamID)
	}
	b.mu.Unlock()
	if ok {
		close(s.toBrowser)
	}
}

func (b *Broker) handleInbound(deviceID uuid.UUID, kind byte, streamID uuid.UUID, payload []byte) {
	b.mu.Lock()
	s, ok := b.streams[streamID]
	b.mu.Unlock()
	if !ok || s.DeviceID != deviceID || s.Kind != kind {
		return // unknown/mismatched stream: silently drop, never trust unsolicited stream data
	}
	// Never block the shared hub read loop on a slow browser; drop the
	// oldest-style by attempting a non-blocking send only.
	select {
	case s.toBrowser <- payload:
	default:
	}
}
