package auth

import (
	"sync"
	"time"
)

// RateLimiter is a simple fixed-window limiter keyed by an arbitrary string
// (e.g. "login:<ip>:<username>", "mfa:<user_id>", "enroll:<ip>"). It is
// intentionally in-process; a multi-instance deployment should front it with
// a shared limiter, but a single wr-core instance is the documented V1
// deployment model. See docs/SECURITY.md §10.
type RateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string][]time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, hits: make(map[string][]time.Time)}
}

// Allow reports whether another attempt for key is permitted right now, and
// records the attempt if so. Backoff is preferred over permanent lockout.
func (r *RateLimiter) Allow(key string) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	cutoff := now.Add(-r.window)
	times := r.hits[key]
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= r.limit {
		r.hits[key] = kept
		return false
	}
	kept = append(kept, now)
	r.hits[key] = kept
	return true
}
