package netmetrics

import "sync/atomic"

// ControlBytesCounter tracks bytes exchanged over the control channel
// specifically (as opposed to the device's total network I/O) — the
// "how much traffic does this client make with the server" figure. It
// counts control-channel *message payload* bytes at the point they're
// handed to/read from the websocket, not raw TCP/TLS bytes on the wire,
// so it under-counts actual framing/encryption overhead somewhat; that's
// an accepted approximation, not an exact accounting figure.
//
// Survives reconnects: created once for the agent process lifetime and
// shared across every connection attempt, so a brief disconnect doesn't
// reset or lose the running total between sampler ticks.
type ControlBytesCounter struct {
	sent uint64
	recv uint64
}

func (c *ControlBytesCounter) AddSent(n int) {
	if n > 0 {
		atomic.AddUint64(&c.sent, uint64(n))
	}
}

func (c *ControlBytesCounter) AddRecv(n int) {
	if n > 0 {
		atomic.AddUint64(&c.recv, uint64(n))
	}
}

// SnapshotReset atomically reads and zeroes both counters — used by the
// sampler to get "bytes since the last sample" without a lock.
func (c *ControlBytesCounter) SnapshotReset() (sent, recv uint64) {
	return atomic.SwapUint64(&c.sent, 0), atomic.SwapUint64(&c.recv, 0)
}
