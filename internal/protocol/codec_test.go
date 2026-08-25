package protocol

import "testing"

func TestEncodeDecodeRoundTrip(t *testing.T) {
	raw, err := Encode(TypeHeartbeat, nil, HeartbeatPayload{UptimeSeconds: 42, Sequence: 7})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	env, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if env.Type != TypeHeartbeat {
		t.Fatalf("expected type %q, got %q", TypeHeartbeat, env.Type)
	}
	if env.Protocol != Version {
		t.Fatalf("expected protocol %d, got %d", Version, env.Protocol)
	}

	var hb HeartbeatPayload
	if err := DecodePayload(env, &hb); err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if hb.UptimeSeconds != 42 || hb.Sequence != 7 {
		t.Fatalf("unexpected payload: %+v", hb)
	}
}

func TestDecodeRejectsMissingRequiredFields(t *testing.T) {
	cases := []string{
		`{}`,
		`{"protocol":1}`,
		`{"protocol":1,"type":"heartbeat"}`,
		`{"type":"heartbeat","message_id":"x"}`, // missing protocol
	}
	for _, c := range cases {
		if _, err := Decode([]byte(c)); err == nil {
			t.Fatalf("expected Decode to reject %q", c)
		}
	}
}

func TestDecodeRejectsMalformedJSON(t *testing.T) {
	if _, err := Decode([]byte("not json")); err == nil {
		t.Fatal("expected Decode to reject malformed JSON")
	}
}

func TestMessageIDsAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewMessageID()
		if seen[id] {
			t.Fatalf("duplicate message id generated: %s", id)
		}
		seen[id] = true
	}
}

func TestDecodePayloadRejectsEmptyPayload(t *testing.T) {
	env := Envelope{Protocol: Version, Type: TypeHeartbeat, MessageID: "x"}
	var hb HeartbeatPayload
	if err := DecodePayload(env, &hb); err == nil {
		t.Fatal("expected DecodePayload to reject empty payload")
	}
}

func TestRequestIDIsCarriedThrough(t *testing.T) {
	reqID := "some-request-id"
	raw, err := Encode(TypeCommandResult, &reqID, CommandResultPayload{Status: "success", Code: CodeOK})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	env, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if env.RequestID == nil || *env.RequestID != reqID {
		t.Fatalf("expected request_id %q to round-trip, got %v", reqID, env.RequestID)
	}
}
