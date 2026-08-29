// Package netmetrics is the agent-side local buffer for network traffic
// samples. It exists because the agent already has plenty of metrics
// (CPU/RAM/disk) that it simply collects and pushes live on every status
// tick — but network traffic is naturally a high-frequency, small-sample
// metric (sampled far more often than the 5-minute status interval to get
// a usable graph) that would either spam the control channel with tiny
// messages or lose all resolution if forced onto the same cadence. So
// instead: sample locally into this on-disk buffer often, then flush it
// to the server in one batch per upload interval and delete what was
// successfully sent — which also means a period offline doesn't lose the
// data, it just uploads late.
//
// Uses modernc.org/sqlite (pure Go, no cgo) rather than mattn/go-sqlite3
// specifically to keep the agent's documented one-line cross-compile
// (`GOOS=... go build ./cmd/wr-agent`, README.md) working unchanged.
package netmetrics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Sample is one local network-traffic observation covering the period
// since the previous sample (IntervalSeconds may not be exactly the
// configured sample interval — the agent could have been busy, suspended,
// or just starting up — so it's stored explicitly rather than assumed,
// letting the server compute an accurate rate instead of a nominal one).
type Sample struct {
	// ID is the local rowid; zero until inserted. Only used to identify
	// which rows a batch upload may delete once the server has them.
	ID               int64
	OccurredAt       time.Time
	IntervalSeconds  float64
	BytesSentTotal   uint64
	BytesRecvTotal   uint64
	BytesSentControl uint64
	BytesRecvControl uint64
}

// Store is a single-file local buffer of not-yet-uploaded Samples.
type Store struct {
	db *sql.DB
}

// Open creates (if needed) and opens the local buffer database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("netmetrics: open %s: %w", path, err)
	}
	// This file is only ever touched by one process (this agent), from
	// two goroutines (sampler writes, uploader reads+deletes) — a single
	// connection avoids any need to reason about sqlite's file-locking
	// behavior under concurrent connections from the same process.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS samples (
			id                  INTEGER PRIMARY KEY AUTOINCREMENT,
			occurred_at         TEXT NOT NULL,
			interval_seconds    REAL NOT NULL,
			bytes_sent_total    INTEGER NOT NULL,
			bytes_recv_total    INTEGER NOT NULL,
			bytes_sent_control  INTEGER NOT NULL,
			bytes_recv_control  INTEGER NOT NULL
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("netmetrics: create schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Insert records one local sample.
func (s *Store) Insert(ctx context.Context, sample Sample) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO samples (occurred_at, interval_seconds, bytes_sent_total, bytes_recv_total, bytes_sent_control, bytes_recv_control)
		VALUES (?,?,?,?,?,?)
	`, sample.OccurredAt.UTC().Format(time.RFC3339Nano), sample.IntervalSeconds,
		int64(sample.BytesSentTotal), int64(sample.BytesRecvTotal), int64(sample.BytesSentControl), int64(sample.BytesRecvControl))
	if err != nil {
		return fmt.Errorf("netmetrics: insert sample: %w", err)
	}
	return nil
}

// Pending returns up to limit not-yet-uploaded samples, oldest first.
func (s *Store) Pending(ctx context.Context, limit int) ([]Sample, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, occurred_at, interval_seconds, bytes_sent_total, bytes_recv_total, bytes_sent_control, bytes_recv_control
		FROM samples ORDER BY id ASC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("netmetrics: query pending: %w", err)
	}
	defer rows.Close()

	var out []Sample
	for rows.Next() {
		var sample Sample
		var occurredAt string
		var sentTotal, recvTotal, sentControl, recvControl int64
		if err := rows.Scan(&sample.ID, &occurredAt, &sample.IntervalSeconds, &sentTotal, &recvTotal, &sentControl, &recvControl); err != nil {
			return nil, fmt.Errorf("netmetrics: scan pending: %w", err)
		}
		sample.BytesSentTotal, sample.BytesRecvTotal = uint64(sentTotal), uint64(recvTotal)
		sample.BytesSentControl, sample.BytesRecvControl = uint64(sentControl), uint64(recvControl)
		sample.OccurredAt, _ = time.Parse(time.RFC3339Nano, occurredAt)
		out = append(out, sample)
	}
	return out, rows.Err()
}

// DeleteUpTo removes every sample with id <= maxID — called once a batch
// up to and including that id has been successfully handed to the server.
func (s *Store) DeleteUpTo(ctx context.Context, maxID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM samples WHERE id <= ?`, maxID); err != nil {
		return fmt.Errorf("netmetrics: delete uploaded: %w", err)
	}
	return nil
}
