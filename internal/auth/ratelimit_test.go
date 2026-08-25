package auth

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToLimit(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("k") {
			t.Fatalf("expected attempt %d to be allowed", i)
		}
	}
	if rl.Allow("k") {
		t.Fatal("expected 4th attempt within window to be denied")
	}
}

func TestRateLimiterIsolatesKeys(t *testing.T) {
	rl := NewRateLimiter(1, time.Minute)
	if !rl.Allow("a") {
		t.Fatal("expected first attempt for key a to be allowed")
	}
	if !rl.Allow("b") {
		t.Fatal("expected key b to be independent of key a")
	}
}

func TestRateLimiterResetsAfterWindow(t *testing.T) {
	rl := NewRateLimiter(1, 20*time.Millisecond)
	if !rl.Allow("k") {
		t.Fatal("expected first attempt to be allowed")
	}
	if rl.Allow("k") {
		t.Fatal("expected second immediate attempt to be denied")
	}
	time.Sleep(30 * time.Millisecond)
	if !rl.Allow("k") {
		t.Fatal("expected attempt after window expiry to be allowed again")
	}
}
