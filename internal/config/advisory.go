package config

import "strings"

// Advisory is a non-blocking configuration recommendation surfaced to
// admins — never something that fails startup (that's what Validate is
// for), just a "you should probably fix this" nudge shown in the
// dashboard and logged at startup. Deliberately config-based rather than
// trying to probe the network at runtime: wr-core never terminates TLS
// itself (docs/DEPLOYMENT.md §4), so it can't directly observe whether a
// reverse proxy in front of it is actually doing HTTPS — but a
// public.base_url that isn't https:// is a reliable, honest signal that
// nobody has set one up yet.
type Advisory struct {
	Code     string `json:"code"`
	Severity string `json:"severity"` // "info" | "warning" | "critical"
	Message  string `json:"message"`
}

// SecurityAdvisories returns configuration-derived recommendations —
// currently just the HTTPS check, but the shape leaves room to add more
// (e.g. a future check for an overly long session TTL) without changing
// callers.
func (c ServerConfig) SecurityAdvisories() []Advisory {
	var out []Advisory

	if !strings.HasPrefix(c.Public.BaseURL, "https://") {
		severity := "warning"
		msg := "public.base_url is not HTTPS — traffic between agents/browsers and this server is unencrypted on the wire. Set up a TLS-terminating reverse proxy (see docs/DEPLOYMENT.md) and update public.base_url."
		if c.Mode == "production" {
			severity = "critical"
			msg = "Running in production mode without HTTPS (public.base_url is not https://) — set up a TLS-terminating reverse proxy before exposing this server, see docs/DEPLOYMENT.md."
		}
		out = append(out, Advisory{Code: "insecure_base_url", Severity: severity, Message: msg})
	}

	if c.Mode != "production" {
		out = append(out, Advisory{
			Code: "development_mode", Severity: "info",
			Message: "Server is running in development mode — some security defaults (session/TOTP secrets, cookie/CSRF/HSTS enforcement) are relaxed for convenience. Set mode: production for a real deployment.",
		})
	}

	return out
}
