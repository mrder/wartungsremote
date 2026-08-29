package netmetrics

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "netmetrics.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertAndPendingRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	want := Sample{
		OccurredAt:       time.Now().UTC().Truncate(time.Second),
		IntervalSeconds:  60.5,
		BytesSentTotal:   1234,
		BytesRecvTotal:   5678,
		BytesSentControl: 90,
		BytesRecvControl: 12,
	}
	if err := s.Insert(ctx, want); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := s.Pending(ctx, 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 pending sample, got %d", len(got))
	}
	if got[0].ID == 0 {
		t.Fatal("expected a nonzero assigned ID")
	}
	if !got[0].OccurredAt.Equal(want.OccurredAt) {
		t.Fatalf("OccurredAt mismatch: got %v, want %v", got[0].OccurredAt, want.OccurredAt)
	}
	if got[0].IntervalSeconds != want.IntervalSeconds ||
		got[0].BytesSentTotal != want.BytesSentTotal || got[0].BytesRecvTotal != want.BytesRecvTotal ||
		got[0].BytesSentControl != want.BytesSentControl || got[0].BytesRecvControl != want.BytesRecvControl {
		t.Fatalf("field mismatch: got %+v, want %+v", got[0], want)
	}
}

func TestPendingOrderedOldestFirst(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := s.Insert(ctx, Sample{OccurredAt: time.Now().UTC(), IntervalSeconds: 60, BytesSentTotal: uint64(i)}); err != nil {
			t.Fatalf("Insert %d: %v", i, err)
		}
	}

	got, err := s.Pending(ctx, 100)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(got))
	}
	for i, s := range got {
		if s.BytesSentTotal != uint64(i) {
			t.Fatalf("expected ascending order by insertion, got %+v at index %d", s, i)
		}
	}
}

func TestPendingRespectsLimit(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		if err := s.Insert(ctx, Sample{OccurredAt: time.Now().UTC(), IntervalSeconds: 60}); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}
	got, err := s.Pending(ctx, 3)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected limit of 3, got %d", len(got))
	}
}

func TestDeleteUpToOnlyRemovesUpToGivenID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if err := s.Insert(ctx, Sample{OccurredAt: time.Now().UTC(), IntervalSeconds: 60}); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}
	all, err := s.Pending(ctx, 100)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 samples before delete, got %d", len(all))
	}

	// Delete the first two.
	if err := s.DeleteUpTo(ctx, all[1].ID); err != nil {
		t.Fatalf("DeleteUpTo: %v", err)
	}

	remaining, err := s.Pending(ctx, 100)
	if err != nil {
		t.Fatalf("Pending after delete: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining samples, got %d", len(remaining))
	}
	if remaining[0].ID != all[2].ID || remaining[1].ID != all[3].ID {
		t.Fatalf("expected the last two original rows to survive, got ids %d,%d", remaining[0].ID, remaining[1].ID)
	}
}

func TestControlBytesCounterSnapshotResetsToZero(t *testing.T) {
	c := &ControlBytesCounter{}
	c.AddSent(100)
	c.AddRecv(200)
	c.AddSent(50)

	sent, recv := c.SnapshotReset()
	if sent != 150 || recv != 200 {
		t.Fatalf("expected sent=150 recv=200, got sent=%d recv=%d", sent, recv)
	}

	sent, recv = c.SnapshotReset()
	if sent != 0 || recv != 0 {
		t.Fatalf("expected counters to reset to 0 after snapshot, got sent=%d recv=%d", sent, recv)
	}
}
