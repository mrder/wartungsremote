package protocol

import (
	"fmt"

	"github.com/google/uuid"
)

// StreamFrameHeaderLen is the fixed header size before the payload, per
// docs/PROTOCOL.md §11:
//
//	byte 0      frame_version = 1
//	byte 1      stream_kind
//	bytes 2-17  stream_id (UUID, 16 bytes binary)
//	bytes 18..  payload
const StreamFrameHeaderLen = 18

const streamFrameVersion = 1

// EncodeStreamFrame wraps payload with the binary stream header used for
// terminal/tunnel/file bulk data on the control channel.
func EncodeStreamFrame(kind byte, streamID uuid.UUID, payload []byte) []byte {
	out := make([]byte, StreamFrameHeaderLen+len(payload))
	out[0] = streamFrameVersion
	out[1] = kind
	copy(out[2:18], streamID[:])
	copy(out[18:], payload)
	return out
}

// DecodeStreamFrame parses a binary stream frame produced by EncodeStreamFrame.
func DecodeStreamFrame(frame []byte) (kind byte, streamID uuid.UUID, payload []byte, err error) {
	if len(frame) < StreamFrameHeaderLen {
		return 0, uuid.Nil, nil, fmt.Errorf("protocol: stream frame too short (%d bytes)", len(frame))
	}
	if frame[0] != streamFrameVersion {
		return 0, uuid.Nil, nil, fmt.Errorf("protocol: unsupported stream frame version %d", frame[0])
	}
	kind = frame[1]
	id, err := uuid.FromBytes(frame[2:18])
	if err != nil {
		return 0, uuid.Nil, nil, fmt.Errorf("protocol: decode stream id: %w", err)
	}
	return kind, id, frame[18:], nil
}
