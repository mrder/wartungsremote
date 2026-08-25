package monitoring

import (
	"testing"
	"time"

	"wartungsremote/internal/device"
)

func TestIsOlderVersion(t *testing.T) {
	cases := []struct {
		version, minimum string
		wantOlder        bool
	}{
		{"0.1.0", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.2.0", "1.0.0", false},
		{"1.0.9", "1.0.10", true},
		{"2.0.0", "1.9.9", false},
	}
	for _, c := range cases {
		got := IsOlderVersion(c.version, c.minimum)
		if got != c.wantOlder {
			t.Errorf("IsOlderVersion(%q, %q) = %v, want %v", c.version, c.minimum, got, c.wantOlder)
		}
	}
}

func TestIsOlderVersionFailsOpenOnMalformedInput(t *testing.T) {
	// A malformed version string must never be treated as "older" (which
	// would otherwise turn a parsing bug into a spurious health warning);
	// only the well-formed comparison path should ever flag it.
	if IsOlderVersion("not-a-version", "1.0.0") {
		t.Fatal("expected malformed version to fail open (not flagged as older)")
	}
}

func TestSustainedAboveRequiresFullWindowCoverage(t *testing.T) {
	now := time.Now().UTC()
	always := func(device.MetricsPoint) bool { return true }

	// Points span less than the required window.
	short := []device.MetricsPoint{
		{ObservedAt: now.Add(-2 * time.Minute)},
		{ObservedAt: now},
	}
	if sustainedAbove(short, 10*time.Minute, always) {
		t.Fatal("expected short window coverage to not count as sustained")
	}

	full := []device.MetricsPoint{
		{ObservedAt: now.Add(-10 * time.Minute)},
		{ObservedAt: now.Add(-5 * time.Minute)},
		{ObservedAt: now},
	}
	if !sustainedAbove(full, 10*time.Minute, always) {
		t.Fatal("expected full window coverage to count as sustained")
	}
}

func TestSustainedAboveClearsOnAnySampleBelowThreshold(t *testing.T) {
	now := time.Now().UTC()
	i := 0
	// second call (the low sample) returns false
	pred := func(device.MetricsPoint) bool {
		i++
		return i != 2
	}
	points := []device.MetricsPoint{
		{ObservedAt: now.Add(-10 * time.Minute)},
		{ObservedAt: now.Add(-5 * time.Minute)},
		{ObservedAt: now},
	}
	if sustainedAbove(points, 10*time.Minute, pred) {
		t.Fatal("expected a single below-threshold sample to clear the sustained condition")
	}
}

func TestSustainedAboveEmptyHistory(t *testing.T) {
	if sustainedAbove(nil, time.Minute, func(device.MetricsPoint) bool { return true }) {
		t.Fatal("expected empty history to never count as sustained")
	}
}
