package netmetrics

import (
	"context"
	"log/slog"
	"time"
)

// SampleInterval is fixed rather than server-configurable: it only
// affects local disk usage and chart resolution, not server load (unlike
// the upload interval, which the server does control — see
// protocol.HelloAckPayload.NetworkUploadIntervalSeconds), so there's no
// need to plumb it through the handshake.
const SampleInterval = 60 * time.Second

// TotalCounters reads cumulative (since-boot) system-wide network byte
// counters. Implemented per-OS in internal/platform.
type TotalCounters interface {
	NetworkCounters(ctx context.Context) (bytesSent, bytesRecv uint64, err error)
}

// RunSampler ticks every SampleInterval for the lifetime of ctx, reading
// system-wide network counters and the shared control-channel counter,
// and inserting one delta row into store per tick. Runs independently of
// any control-channel connection — sampling continues (and buffers up)
// even while disconnected.
func RunSampler(ctx context.Context, store *Store, counters TotalCounters, control *ControlBytesCounter) {
	ticker := time.NewTicker(SampleInterval)
	defer ticker.Stop()

	lastSent, lastRecv, ok := readTotals(ctx, counters)
	lastAt := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		now := time.Now()
		sent, recv, readOK := readTotals(ctx, counters)
		controlSent, controlRecv := control.SnapshotReset()

		var deltaSent, deltaRecv uint64
		if ok && readOK && sent >= lastSent && recv >= lastRecv {
			deltaSent, deltaRecv = sent-lastSent, recv-lastRecv
		}
		// A negative delta (counter reset — interface reconnected, or the
		// OS wrapped a 32-bit counter) is treated as "unknown for this
		// tick" (zero) rather than negative/nonsense; the next tick's
		// baseline is still whatever the OS reports now, so it recovers
		// on its own.
		if readOK {
			lastSent, lastRecv, ok = sent, recv, true
		}

		if err := store.Insert(ctx, Sample{
			OccurredAt:       now,
			IntervalSeconds:  now.Sub(lastAt).Seconds(),
			BytesSentTotal:   deltaSent,
			BytesRecvTotal:   deltaRecv,
			BytesSentControl: controlSent,
			BytesRecvControl: controlRecv,
		}); err != nil {
			slog.Warn("failed to record local network sample", "error", err)
		}
		lastAt = now
	}
}

func readTotals(ctx context.Context, counters TotalCounters) (sent, recv uint64, ok bool) {
	sent, recv, err := counters.NetworkCounters(ctx)
	if err != nil {
		slog.Warn("failed to read network counters", "error", err)
		return 0, 0, false
	}
	return sent, recv, true
}
