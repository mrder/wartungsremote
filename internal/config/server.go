// Package config loads and validates server and agent configuration per
// docs/CONFIGURATION.md. Security defaults must never be silently weakened
// by empty/missing values.
package config

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"wartungsremote/internal/netutil"
)

// ServerConfig is the top-level wr-core configuration.
type ServerConfig struct {
	Mode string `yaml:"mode"` // "production" | "development"

	Public struct {
		BaseURL string `yaml:"base_url"`
		Listen  string `yaml:"listen"`
		// DeterrentContactURL, if set, is shown as a link on the public
		// listener's deterrent page (someone opening the bare hostname by
		// mistake). Empty by default — deliberately not hardcoded to any
		// one deployment's own dashboard URL, since that would be wrong
		// for every other deployment of this same source.
		DeterrentContactURL string `yaml:"deterrent_contact_url"`
	} `yaml:"public"`

	Admin struct {
		Listen             string        `yaml:"listen"`
		SessionAbsoluteTTL time.Duration `yaml:"session_absolute_ttl"`
		SessionIdleTTL     time.Duration `yaml:"session_idle_ttl"`
		PrivilegeTTL       time.Duration `yaml:"privilege_ttl"`
		RequireMFA         bool          `yaml:"require_mfa"`
	} `yaml:"admin"`

	Agent struct {
		HeartbeatInterval   time.Duration `yaml:"heartbeat_interval"`
		ConnectionLostAfter time.Duration `yaml:"connection_lost_after"`
		OfflineAfter        time.Duration `yaml:"offline_after"`
		StatusInterval      time.Duration `yaml:"status_interval"`
		// NetworkUploadInterval controls how often an agent flushes its
		// locally-buffered network traffic samples (docs/AGENT.md
		// "Netzwerk-Traffic-Metriken") — deliberately separate from
		// StatusInterval since network samples are collected agent-side
		// far more often (every internal/netmetrics.SampleInterval) than
		// they're uploaded; this just governs upload frequency/server load.
		NetworkUploadInterval time.Duration `yaml:"network_upload_interval"`
		EnrollmentTTL         time.Duration `yaml:"enrollment_ttl"`
		ReconnectMaxBackoff   time.Duration `yaml:"reconnect_max_backoff"`
		// GitHubReleaseSync periodically imports agent-* GitHub Releases as
		// agent_versions rows (docs/AGENT.md §15) instead of requiring manual
		// entry through the dashboard. Every import still goes through
		// agentrelease.Repo.Create's Ed25519 signature verification against
		// the same trusted offline key as the manual form — GitHub is only
		// ever a transport, never a trust source. Interval 0 disables sync
		// entirely (the manual form keeps working either way).
		GitHubRepo             string        `yaml:"github_repo"`
		GitHubReleaseSyncEvery time.Duration `yaml:"github_release_sync_interval"`
	} `yaml:"agent"`

	Relay struct {
		TicketTTL           time.Duration `yaml:"ticket_ttl"`
		MaxTunnelsPerUser   int           `yaml:"max_tunnels_per_user"`
		MaxTunnelsPerDevice int           `yaml:"max_tunnels_per_device"`
		MaxSessionDuration  time.Duration `yaml:"max_session_duration"`
	} `yaml:"relay"`

	Security struct {
		SessionCookieName string `yaml:"session_cookie_name"`
		CSRFEnabled       bool   `yaml:"csrf_enabled"`
		HSTSEnabled       bool   `yaml:"hsts_enabled"`
		// Argon2id parameters, benchmarkable/configurable per docs/SECURITY.md §7.
		Argon2MemoryKiB   uint32 `yaml:"argon2_memory_kib"`
		Argon2Iterations  uint32 `yaml:"argon2_iterations"`
		Argon2Parallelism uint8  `yaml:"argon2_parallelism"`
		// TrustedProxies lists the only IPs/CIDRs allowed to set
		// X-Forwarded-For when determining a caller's IP for audit/device
		// tracking purposes (e.g. a reverse proxy container's address on
		// the same Docker network). Empty (the default) means nobody is
		// trusted and X-Forwarded-For is always ignored — the raw TCP
		// peer address is used instead. Never add an entry here unless a
		// proxy actually sits at that exact address, since anything in
		// this list can claim any IP it wants on behalf of a caller.
		TrustedProxies []string `yaml:"trusted_proxies"`
	} `yaml:"security"`

	Metrics struct {
		RawRetention    time.Duration `yaml:"raw_retention"`
		HourlyRetention time.Duration `yaml:"hourly_retention"`
		// Network*Retention govern device_network_metrics[_hourly]
		// separately from the above, since traffic samples are collected
		// at a much higher frequency (every internal/netmetrics.SampleInterval)
		// than the CPU/RAM/disk status reports the above two apply to.
		NetworkRawRetention    time.Duration `yaml:"network_raw_retention"`
		NetworkHourlyRetention time.Duration `yaml:"network_hourly_retention"`
	} `yaml:"metrics"`

	// Help points at the directory containing DASHBOARD_HELP.md, rendered
	// as the in-dashboard help pages (docs/API.md §19).
	Help struct {
		ContentDir string `yaml:"content_dir"`
	} `yaml:"help"`

	// Secrets are never read from the YAML file body directly; they are
	// resolved from *_FILE environment variables. See docs/CONFIGURATION.md §2.
	Secrets Secrets `yaml:"-"`
}

// Secrets holds resolved secret material, sourced exclusively from files
// referenced by environment variables (never inline config, never plain env vars).
type Secrets struct {
	DatabaseURL string
	// MigrationDatabaseURL, if set, is used only for running schema
	// migrations at startup (needs DDL rights); DatabaseURL is then free
	// to be a least-privilege runtime role that can't ALTER TABLE
	// (docs/DEPLOYMENT.md §5a). Falls back to DatabaseURL when unset —
	// the historical single-DSN behavior, unchanged for native/dev
	// installs that don't use the restricted role at all.
	MigrationDatabaseURL string
	SessionPepper        []byte
	TOTPEncryptionKey    []byte
	InternalServiceKey   []byte
	// ReleasePublicKey is the Ed25519 public key used to verify agent
	// release signatures (docs/AGENT.md §15). The corresponding private
	// key never touches the server — releases are signed offline with
	// wr-release-sign. Not secret in the confidentiality sense, but
	// resolved the same way so the trusted key is provisioned explicitly
	// rather than hardcoded.
	ReleasePublicKey []byte
	// GitHubToken is optional — only needed to raise the unauthenticated
	// GitHub API rate limit or to sync releases from a private repo. Sync
	// works fine without it for a public repo at normal usage volumes.
	GitHubToken string
}

// Default returns the documented safe defaults from docs/CONFIGURATION.md §1.
func Default() ServerConfig {
	var c ServerConfig
	c.Mode = "production"
	c.Public.BaseURL = "https://localhost:8443"
	c.Public.Listen = "0.0.0.0:8443"
	c.Admin.Listen = "127.0.0.1:9443"
	c.Admin.SessionAbsoluteTTL = 8 * time.Hour
	c.Admin.SessionIdleTTL = 30 * time.Minute
	c.Admin.PrivilegeTTL = 15 * time.Minute
	c.Admin.RequireMFA = true
	c.Agent.HeartbeatInterval = 45 * time.Second
	c.Agent.ConnectionLostAfter = 120 * time.Second
	c.Agent.OfflineAfter = 300 * time.Second
	c.Agent.StatusInterval = 5 * time.Minute
	c.Agent.NetworkUploadInterval = 5 * time.Minute
	c.Agent.EnrollmentTTL = 30 * time.Minute
	c.Agent.ReconnectMaxBackoff = 5 * time.Minute
	c.Agent.GitHubRepo = "mrder/wartungsremote"
	// Disabled by default — this reaches out to the internet on its own
	// initiative, which should be an explicit opt-in (github_release_sync_interval
	// in server.yaml) rather than a silent behavior change on upgrade.
	c.Agent.GitHubReleaseSyncEvery = 0
	c.Relay.TicketTTL = 60 * time.Second
	c.Relay.MaxTunnelsPerUser = 5
	c.Relay.MaxTunnelsPerDevice = 3
	c.Relay.MaxSessionDuration = 8 * time.Hour
	c.Security.SessionCookieName = "__Host-wr_session"
	c.Security.CSRFEnabled = true
	c.Security.HSTSEnabled = true
	// OWASP Password Storage Cheat Sheet Argon2id minimum baseline (m=19 MiB, t=2, p=1).
	c.Security.Argon2MemoryKiB = 19 * 1024
	c.Security.Argon2Iterations = 2
	c.Security.Argon2Parallelism = 1
	c.Metrics.RawRetention = 30 * 24 * time.Hour
	c.Metrics.HourlyRetention = 365 * 24 * time.Hour
	// Shorter raw default than CPU/RAM/disk: at one sample/minute per
	// device (vs. one per 5 minutes), 30 days would be 5x the row volume
	// for comparable resolution depth; 7 days of raw plus a year of
	// hourly rollups covers both "what happened this week" and long-term
	// trend without that cost.
	c.Metrics.NetworkRawRetention = 7 * 24 * time.Hour
	c.Metrics.NetworkHourlyRetention = 365 * 24 * time.Hour
	c.Help.ContentDir = "docs"
	return c
}

// LoadServer reads defaults, overlays an optional YAML file, then resolves
// secrets from *_FILE environment variables, per the documented priority:
// Defaults < config file < environment non-secret overrides < secret files.
func LoadServer(path string) (ServerConfig, error) {
	c := Default()

	if path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return c, fmt.Errorf("config: read %s: %w", path, err)
			}
		} else if err := yaml.Unmarshal(body, &c); err != nil {
			return c, fmt.Errorf("config: parse %s: %w", path, err)
		}
	}

	secrets, err := loadSecrets()
	if err != nil {
		return c, err
	}
	c.Secrets = secrets

	if c.Mode == "development" {
		// Never do this in production: ephemeral secrets mean every restart
		// invalidates all sessions and TOTP enrollments. This exists purely
		// so `mode: development` works without hand-provisioning secret
		// files, matching docs/CONFIGURATION.md §5 (only production start-up
		// is required to fail on missing secrets).
		if err := c.fillDevelopmentSecrets(); err != nil {
			return c, err
		}
	}

	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

func (c *ServerConfig) fillDevelopmentSecrets() error {
	if len(c.Secrets.SessionPepper) < 32 {
		b, err := randomBytes(32)
		if err != nil {
			return err
		}
		c.Secrets.SessionPepper = b
		slog.Warn("WR_SESSION_PEPPER_FILE not set; using an ephemeral development-only session pepper (sessions will not survive a restart)")
	}
	if len(c.Secrets.TOTPEncryptionKey) != 32 {
		b, err := randomBytes(32)
		if err != nil {
			return err
		}
		c.Secrets.TOTPEncryptionKey = b
		slog.Warn("WR_TOTP_ENCRYPTION_KEY_FILE not set; using an ephemeral development-only TOTP key (existing TOTP enrollments will stop validating after a restart)")
	}
	return nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("config: generate ephemeral secret: %w", err)
	}
	return b, nil
}

func loadSecrets() (Secrets, error) {
	var s Secrets

	dbURL, err := readSecretFile("WR_DATABASE_URL_FILE")
	if err != nil {
		return s, err
	}
	s.DatabaseURL = string(dbURL)

	migrationDBURL, err := readSecretFile("WR_MIGRATION_DATABASE_URL_FILE")
	if err != nil {
		return s, err
	}
	s.MigrationDatabaseURL = string(migrationDBURL)

	pepper, err := readSecretFile("WR_SESSION_PEPPER_FILE")
	if err != nil {
		return s, err
	}
	s.SessionPepper = pepper

	totpKey, err := readSecretFile("WR_TOTP_ENCRYPTION_KEY_FILE")
	if err != nil {
		return s, err
	}
	s.TOTPEncryptionKey = totpKey

	svcKey, err := readSecretFile("WR_INTERNAL_SERVICE_KEY_FILE")
	if err != nil {
		return s, err
	}
	s.InternalServiceKey = svcKey

	releaseKey, err := readSecretFile("WR_RELEASE_PUBLIC_KEY_FILE")
	if err != nil {
		return s, err
	}
	s.ReleasePublicKey = releaseKey

	githubToken, err := readSecretFile("WR_GITHUB_TOKEN_FILE")
	if err != nil {
		return s, err
	}
	s.GitHubToken = string(githubToken)

	return s, nil
}

func readSecretFile(envVar string) ([]byte, error) {
	path := os.Getenv(envVar)
	if path == "" {
		return nil, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read secret %s (%s): %w", envVar, path, err)
	}
	return trimTrailingNewline(body), nil
}

func trimTrailingNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

// Validate enforces the documented start-time refusal rules (§5): production
// mode must never start with an insecure configuration.
func (c ServerConfig) Validate() error {
	if c.Mode != "production" && c.Mode != "development" {
		return fmt.Errorf("config: invalid mode %q", c.Mode)
	}
	if c.Admin.SessionAbsoluteTTL <= 0 || c.Admin.SessionIdleTTL <= 0 || c.Admin.PrivilegeTTL <= 0 {
		return fmt.Errorf("config: session/privilege TTLs must be positive")
	}
	if c.Agent.HeartbeatInterval <= 0 || c.Agent.ConnectionLostAfter <= 0 || c.Agent.OfflineAfter <= 0 {
		return fmt.Errorf("config: agent timing values must be positive")
	}
	if c.Agent.OfflineAfter <= c.Agent.ConnectionLostAfter {
		return fmt.Errorf("config: offline_after must be greater than connection_lost_after")
	}
	if _, err := netutil.ParseTrustedProxies(c.Security.TrustedProxies); err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if c.Mode == "production" {
		if !c.Admin.RequireMFA {
			return fmt.Errorf("config: production mode requires admin.require_mfa=true")
		}
		if !c.Security.HSTSEnabled || !c.Security.CSRFEnabled {
			return fmt.Errorf("config: production mode requires hsts_enabled and csrf_enabled")
		}
		if len(c.Security.SessionCookieName) < 6 || c.Security.SessionCookieName[:6] != "__Host" {
			return fmt.Errorf("config: production mode requires a __Host- prefixed session cookie name")
		}
		if c.Secrets.DatabaseURL == "" {
			return fmt.Errorf("config: production mode requires WR_DATABASE_URL_FILE")
		}
		if len(c.Secrets.SessionPepper) < 32 {
			return fmt.Errorf("config: production mode requires WR_SESSION_PEPPER_FILE with >= 32 bytes")
		}
		if len(c.Secrets.TOTPEncryptionKey) != 32 {
			return fmt.Errorf("config: production mode requires WR_TOTP_ENCRYPTION_KEY_FILE with exactly 32 bytes (AES-256)")
		}
	}
	return nil
}
