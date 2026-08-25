package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"

	"wartungsremote/internal/config"
)

// withSecurityHeaders sets the headers required by docs/SECURITY.md §15 on
// every response.
func withSecurityHeaders(cfg config.ServerConfig, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		h.Set("X-Frame-Options", "DENY")
		if cfg.Security.HSTSEnabled {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

const csrfCookieName = "wr_csrf"
const csrfHeaderName = "X-CSRF-Token"

// setCSRFCookie issues a non-HttpOnly double-submit CSRF token alongside a
// new session, per docs/SECURITY.md §9: "CSRF-Tokens für zustandsändernde
// Browserrequests zusätzlich zu SameSite".
func setCSRFCookie(w http.ResponseWriter, cfg config.ServerConfig) {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	token := base64.RawURLEncoding.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		Secure:   true,
		HttpOnly: false,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearCSRFCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: "", Path: "/", MaxAge: -1, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

// withCSRF enforces the double-submit cookie pattern for state-changing
// methods only; GET/HEAD/OPTIONS are exempt as they must not mutate state.
func withCSRF(cfg config.ServerConfig, next http.Handler) http.Handler {
	if !cfg.Security.CSRFEnabled {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || cookie.Value == "" {
			writeErr(w, http.StatusForbidden, "permission_denied", "Missing CSRF token")
			return
		}
		header := r.Header.Get(csrfHeaderName)
		if header == "" || subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) != 1 {
			writeErr(w, http.StatusForbidden, "permission_denied", "Invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}
