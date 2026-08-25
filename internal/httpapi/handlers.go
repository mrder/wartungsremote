package httpapi

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"wartungsremote/internal/agentrelease"
	"wartungsremote/internal/alerting"
	"wartungsremote/internal/audit"
	"wartungsremote/internal/auth"
	"wartungsremote/internal/config"
	"wartungsremote/internal/controlhub"
	"wartungsremote/internal/customer"
	"wartungsremote/internal/device"
	"wartungsremote/internal/enrollment"
	"wartungsremote/internal/help"
	"wartungsremote/internal/maintenance"
	"wartungsremote/internal/monitoring"
	"wartungsremote/internal/relay"
	"wartungsremote/internal/remotesession"
)

type handlers struct {
	cfg          config.ServerConfig
	devices      *device.Repo
	enroll       *enrollment.Service
	auth         *auth.Service
	hub          *controlhub.Hub
	health       *monitoring.Engine
	audit        *audit.Logger
	version      string
	sessions     *remotesession.Service
	sessionRepo  *remotesession.Repo
	privilege    *remotesession.PrivilegeRepo
	tunnels      *remotesession.TunnelRepo
	broker       *relay.Broker
	privilegeTTL time.Duration
	maintenance  *maintenance.Repo
	customers    *customer.Repo
	alerts       *alerting.Repo
	releases     *agentrelease.Repo
	help         []help.Section
}

// agentRejectionReason strips remotesession's internal error-wrapping
// prefix, leaving just the agent-reported reason (e.g. "failed to reach
// local target") for display to the user.
func agentRejectionReason(err error) string {
	return strings.TrimPrefix(err.Error(), remotesession.ErrAgentRejected.Error()+": ")
}

func writeJSON(w http.ResponseWriter, status int, data any, meta any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	body := map[string]any{"data": data}
	if meta != nil {
		body["meta"] = meta
	}
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code, message string) {
	auth.WriteError(w, status, code, message)
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// clientIP returns a bare IP address suitable for storage in a Postgres
// `inet` column. r.RemoteAddr is always host:port, and X-Forwarded-For may
// carry a comma-separated proxy chain, so both must be normalized before
// use — an unparsed "host:port" value fails inet insertion outright.
func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		first := strings.TrimSpace(strings.Split(xf, ",")[0])
		if first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
