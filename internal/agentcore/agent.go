// Package agentcore implements the cross-platform wr-agent core: identity,
// enrollment, the control-channel client with reconnect/backoff, and
// dispatch into the OS-specific platform.Provider. See docs/AGENT.md.
package agentcore

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"wartungsremote/internal/config"
	"wartungsremote/internal/platform"
)

const (
	minBackoff = 1 * time.Second
	maxBackoff = 300 * time.Second
)

// Run drives the reconnect loop described in docs/PROTOCOL.md §15 and
// docs/AGENT.md §7: exponential backoff with jitter, reset on success, no
// aggressive tight-looping. It blocks until ctx is cancelled.
//
// onFirstConnect, if non-nil, fires exactly once per process — the first
// time the control channel handshake succeeds — regardless of how many
// times the connection is later lost and re-established. It exists so
// cmd/wr-agent can commit a pending self-update marker (docs/AGENT.md §15
// step 10 "Health Signal") as soon as the post-update binary proves it can
// actually talk to the server, without tangling that concern into the
// per-reconnect backoff-reset callback below.
func Run(ctx context.Context, serverURL, agentVersion string, identity Identity, provider platform.Provider, policy config.AgentPolicy, dataDir string, onFirstConnect func()) {
	backoff := minBackoff
	var firstConnectOnce sync.Once
	for {
		if ctx.Err() != nil {
			return
		}
		connected := false
		err := runSession(ctx, serverURL, agentVersion, identity, provider, policy, dataDir, func() {
			connected = true
			backoff = minBackoff
			if onFirstConnect != nil {
				firstConnectOnce.Do(onFirstConnect)
			}
		})
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			// graceful shutdown signalled from within runSession
			return
		}

		slog.Warn("control channel session ended, will reconnect", "error", err, "backoff", backoff)
		wait := jitter(backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		if !connected {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// jitter applies +/-20% jitter per docs/PROTOCOL.md §15.
func jitter(d time.Duration) time.Duration {
	delta := float64(d) * 0.2
	offset := (rand.Float64()*2 - 1) * delta
	result := time.Duration(float64(d) + offset)
	if result < 0 {
		result = 0
	}
	return result
}
