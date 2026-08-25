package protocol

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NewMessageID returns a new random message identifier (UUIDv4).
func NewMessageID() string {
	return uuid.NewString()
}

// Encode wraps payload into a fully populated Envelope and marshals it to JSON.
func Encode(msgType string, requestID *string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("protocol: marshal payload: %w", err)
	}
	env := Envelope{
		Protocol:  Version,
		Type:      msgType,
		MessageID: NewMessageID(),
		RequestID: requestID,
		Timestamp: time.Now().UTC(),
		Payload:   body,
	}
	out, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("protocol: marshal envelope: %w", err)
	}
	return out, nil
}

// Decode parses raw bytes into an Envelope without decoding the payload.
func Decode(raw []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Envelope{}, fmt.Errorf("protocol: decode envelope: %w", err)
	}
	if env.Protocol == 0 || env.Type == "" || env.MessageID == "" {
		return Envelope{}, fmt.Errorf("protocol: %w: missing required envelope field", ErrInvalidEnvelope)
	}
	return env, nil
}

// DecodePayload unmarshals an envelope's payload into dst.
func DecodePayload(env Envelope, dst any) error {
	if len(env.Payload) == 0 {
		return fmt.Errorf("protocol: %w: empty payload", ErrInvalidEnvelope)
	}
	if err := json.Unmarshal(env.Payload, dst); err != nil {
		return fmt.Errorf("protocol: decode payload for %q: %w", env.Type, err)
	}
	return nil
}

// ErrInvalidEnvelope is returned when a received envelope fails structural validation.
var ErrInvalidEnvelope = fmt.Errorf("invalid envelope")
