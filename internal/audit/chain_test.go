package audit

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestComputeEntryHashIsChained verifies the two properties VerifyChain
// depends on: same fields + same prev_hash always produce the same hash
// (determinism, needed for both Record and a later independent
// VerifyChain run to agree), and changing any single covered field
// changes the hash (tamper-evidence — the whole reason for the chain,
// docs/SECURITY.md §16).
func TestComputeEntryHashIsChained(t *testing.T) {
	actorID := uuid.New()
	base := chainableFields{
		OccurredAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		ActorType:  "user",
		ActorID:    &actorID,
		EventType:  "auth.login.success",
		Result:     "success",
		SourceIP:   "10.0.0.1",
	}

	h1 := computeEntryHash(nil, base)
	h2 := computeEntryHash(nil, base)
	if !bytesEqual(h1, h2) {
		t.Fatal("computeEntryHash is not deterministic for identical inputs")
	}

	withPrev := computeEntryHash(h1, base)
	if bytesEqual(withPrev, h1) {
		t.Fatal("changing prev_hash did not change the resulting hash")
	}

	mutated := base
	mutated.Result = "failure"
	if bytesEqual(computeEntryHash(nil, mutated), h1) {
		t.Fatal("changing Result did not change the resulting hash — tampering would go undetected")
	}

	mutated = base
	mutated.EventType = "auth.login.failure"
	if bytesEqual(computeEntryHash(nil, mutated), h1) {
		t.Fatal("changing EventType did not change the resulting hash — tampering would go undetected")
	}

	mutated = base
	otherActor := uuid.New()
	mutated.ActorID = &otherActor
	if bytesEqual(computeEntryHash(nil, mutated), h1) {
		t.Fatal("changing ActorID did not change the resulting hash — tampering would go undetected")
	}

	mutated = base
	mutated.MetadataJSON = []byte(`{"k":"v"}`)
	if bytesEqual(computeEntryHash(nil, mutated), h1) {
		t.Fatal("changing MetadataJSON did not change the resulting hash — tampering would go undetected")
	}
}

func TestBytesEqual(t *testing.T) {
	if !bytesEqual(nil, nil) {
		t.Fatal("nil should equal nil")
	}
	if bytesEqual([]byte{1, 2}, []byte{1, 2, 3}) {
		t.Fatal("different lengths should not be equal")
	}
	if bytesEqual([]byte{1, 2}, []byte{1, 3}) {
		t.Fatal("different contents should not be equal")
	}
	if !bytesEqual([]byte{1, 2}, []byte{1, 2}) {
		t.Fatal("identical contents should be equal")
	}
}
