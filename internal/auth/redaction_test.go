package auth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestUserJSONNeverLeaksPasswordHash guards against a future handler
// accidentally returning a User struct directly (instead of a
// hand-redacted map) and leaking the Argon2 password hash into an API
// response.
func TestUserJSONNeverLeaksPasswordHash(t *testing.T) {
	u := User{
		ID:           uuid.New(),
		Username:     "admin",
		DisplayName:  "Admin",
		PasswordHash: "$argon2id$v=19$m=19456,t=2,p=1$c29tZXNhbHQ$c29tZWhhc2g",
		Status:       UserActive,
		CreatedAt:    time.Now(),
	}
	data, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "argon2id") || strings.Contains(string(data), u.PasswordHash) {
		t.Fatalf("expected PasswordHash to never appear in JSON, got %s", data)
	}
	if strings.Contains(string(data), "PasswordHash") || strings.Contains(string(data), "password_hash") {
		t.Fatalf("expected no password_hash field at all, got %s", data)
	}
	// Sanity: the fields that ARE meant to be public still round-trip.
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["username"] != "admin" {
		t.Fatalf("expected username to survive marshaling, got %v", decoded["username"])
	}
}

// TestUserSliceJSONNeverLeaksPasswordHash exercises the actual shape used
// by list endpoints ([]User), not just a single value.
func TestUserSliceJSONNeverLeaksPasswordHash(t *testing.T) {
	users := []User{
		{ID: uuid.New(), Username: "a", PasswordHash: "secret-hash-a", Status: UserActive},
		{ID: uuid.New(), Username: "b", PasswordHash: "secret-hash-b", Status: UserDisabled},
	}
	data, err := json.Marshal(users)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-hash") {
		t.Fatalf("expected no password hash in list response, got %s", data)
	}
}
