package netutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClientIPIgnoresUntrustedForwardedFor guards the actual security
// property this package exists for: without an explicitly configured
// trusted proxy, X-Forwarded-For must never be trusted, since it is
// attacker-controlled input (any caller — including a malicious agent —
// can set it to claim any IP it wants).
func TestClientIPIgnoresUntrustedForwardedFor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:54321"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")

	got := ClientIP(r, TrustedProxies{})
	if got != "203.0.113.7" {
		t.Fatalf("expected untrusted caller's raw peer address, got %q", got)
	}
}

func TestClientIPHonorsForwardedForFromTrustedProxy(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"172.18.0.0/16"})
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "172.18.0.5:54321"
	r.Header.Set("X-Forwarded-For", "198.51.100.9, 172.18.0.5")

	got := ClientIP(r, trusted)
	if got != "198.51.100.9" {
		t.Fatalf("expected first hop of X-Forwarded-For from trusted proxy, got %q", got)
	}
}

func TestClientIPFallsBackWithoutForwardedForHeader(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"172.18.0.5"})
	if err != nil {
		t.Fatal(err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "172.18.0.5:54321"

	got := ClientIP(r, trusted)
	if got != "172.18.0.5" {
		t.Fatalf("expected raw peer address when no X-Forwarded-For is present, got %q", got)
	}
}

func TestParseTrustedProxiesRejectsGarbage(t *testing.T) {
	if _, err := ParseTrustedProxies([]string{"not-an-ip"}); err == nil {
		t.Fatal("expected an error for an invalid trusted proxy entry")
	}
}

func TestParseTrustedProxiesAcceptsBareIPAndCIDR(t *testing.T) {
	if _, err := ParseTrustedProxies([]string{"10.0.0.5", "172.18.0.0/16"}); err != nil {
		t.Fatalf("expected bare IP and CIDR entries to both parse, got: %v", err)
	}
}
