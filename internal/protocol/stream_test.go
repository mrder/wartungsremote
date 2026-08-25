package protocol

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
)

func TestStreamFrameRoundTrip(t *testing.T) {
	id := uuid.New()
	payload := []byte("hello terminal")
	frame := EncodeStreamFrame(StreamKindTerminal, id, payload)

	kind, gotID, gotPayload, err := DecodeStreamFrame(frame)
	if err != nil {
		t.Fatalf("DecodeStreamFrame: %v", err)
	}
	if kind != StreamKindTerminal {
		t.Fatalf("expected kind %d, got %d", StreamKindTerminal, kind)
	}
	if gotID != id {
		t.Fatalf("expected stream id %s, got %s", id, gotID)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatalf("expected payload %q, got %q", payload, gotPayload)
	}
}

func TestStreamFrameRejectsShortFrame(t *testing.T) {
	if _, _, _, err := DecodeStreamFrame([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for too-short frame")
	}
}

func TestStreamFrameRejectsUnknownVersion(t *testing.T) {
	frame := EncodeStreamFrame(StreamKindTerminal, uuid.New(), nil)
	frame[0] = 99
	if _, _, _, err := DecodeStreamFrame(frame); err == nil {
		t.Fatal("expected error for unsupported frame version")
	}
}

func TestStreamFrameEmptyPayload(t *testing.T) {
	id := uuid.New()
	frame := EncodeStreamFrame(StreamKindFile, id, nil)
	kind, gotID, payload, err := DecodeStreamFrame(frame)
	if err != nil {
		t.Fatalf("DecodeStreamFrame: %v", err)
	}
	if kind != StreamKindFile || gotID != id || len(payload) != 0 {
		t.Fatalf("unexpected result: kind=%d id=%s payload=%v", kind, gotID, payload)
	}
}
