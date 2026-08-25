// Package netutil provides small networking helpers shared by the HTTP API
// and the agent control channel — currently just trusted-proxy-aware client
// IP resolution (docs/SECURITY.md).
package netutil

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// TrustedProxies is a set of IP prefixes allowed to set X-Forwarded-For.
// The zero value trusts nothing, which is the safe default: without an
// explicitly configured trusted proxy, X-Forwarded-For is attacker-
// controlled input (any agent or browser can set it on its request) and
// must never be used for anything security- or audit-relevant, such as
// the IP recorded for a device or an audit log entry.
type TrustedProxies struct {
	prefixes []netip.Prefix
}

// ParseTrustedProxies parses a list of bare IPs or CIDRs (e.g. "10.0.0.5"
// or "172.18.0.0/16" — typical for a reverse proxy on the same Docker
// network) into a TrustedProxies set.
func ParseTrustedProxies(entries []string) (TrustedProxies, error) {
	var tp TrustedProxies
	for _, e := range entries {
		p, err := parsePrefixOrAddr(strings.TrimSpace(e))
		if err != nil {
			return TrustedProxies{}, fmt.Errorf("netutil: invalid trusted proxy %q: %w", e, err)
		}
		tp.prefixes = append(tp.prefixes, p)
	}
	return tp, nil
}

func parsePrefixOrAddr(s string) (netip.Prefix, error) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p, nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

func (tp TrustedProxies) contains(host string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	for _, p := range tp.prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// ClientIP returns the bare IP address of the request's true origin,
// suitable for storage in a Postgres `inet` column. X-Forwarded-For is
// only honored when r.RemoteAddr belongs to an explicitly configured
// trusted proxy (security.trusted_proxies) — otherwise it is ignored and
// the raw TCP peer address is used, since an untrusted caller (including
// a malicious agent) can set that header to whatever it wants.
func ClientIP(r *http.Request, trusted TrustedProxies) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if trusted.contains(host) {
		if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
			if first := strings.TrimSpace(strings.Split(xf, ",")[0]); first != "" {
				return first
			}
		}
	}
	return host
}
