# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Fixed/added during first real-machine user testing

- Removed the file **Delete** action from the web Files tab (backend
  endpoint stays permission-gated/audited but is no longer reachable from
  the UI) per explicit user request — browsing/download/upload/mkdir
  remain.
- Disk health now distinguishes fixed disks from removable/optical/network
  media (Win32 `GetDriveType` on Windows, `/sys/block/*/removable` on
  Linux): a full USB stick or SD card no longer flips device health to
  critical the way a full system disk does. New `removable` field on
  `DiskInfo`/`FilesystemUsage`.
- `remotesession.OpenTerminal`/`OpenTunnel` collapsed every non-success
  agent reply (local-policy denial, a failed dial to the SSH/RDP target,
  a malformed reply) into one generic "agent rejected — capability or
  local policy" message, discarding the agent's actual reported reason.
  Now propagates it (e.g. "failed to reach local target" when nothing is
  listening on `127.0.0.1:22`/`3389` on the device) — found live-testing
  an SSH tunnel against a machine with no SSH server running.
- Audit log entries only showed `ActorType` ("user"/"agent"/"system"), not
  *which* user, and the web Audit tab didn't render `SourceIP` even
  though the API already returned it. `audit.Entry` now resolves
  `ActorUsername` via a join; the device Audit tab shows both username
  and source IP.
- The control hub only ever recorded a device's public IP once, at
  connect time (`UpdateConnectivity`) — the periodic inventory-driven
  `device_network` history rows (`RecordNetwork`) always got an empty
  string instead of the connection's real remote IP, so the table existed
  but never actually accumulated IP-history data. Fixed by caching the
  remote IP on the `connection` struct and passing it through both call
  sites. New `GET /devices/:id/ip-history` (distinct IPs + first/last
  seen within a lookback window) and a "Public IP" row with a hover
  tooltip on the device Overview tab.

### Added

- Initial monorepo skeleton (`cmd/`, `internal/`, `web/`, `migrations/`,
  `deployment/`, `scripts/`, `tests/`) per `docs/AI_IMPLEMENTATION_GUIDE.md`.
- Shared wire protocol package (`internal/protocol`) implementing `wrp/1`
  envelopes, message types, and error codes from `docs/PROTOCOL.md`.
- PostgreSQL schema and embedded migration runner covering the full data
  model from `docs/DATABASE.md`.
- Admin authentication: Argon2id password hashing, TOTP setup/verification,
  recovery codes, server-side sessions with `__Host-` cookies, CSRF
  double-submit protection, rate limiting, account lockout, and granular
  RBAC with customer/group scoping.
- One-time enrollment tokens with atomic, race-safe consumption.
- Authenticated agent control channel over WebSocket, using an Ed25519
  challenge/response handshake bound to the device's enrollment-issued
  public key (device identity, not just a device ID, authenticates the
  connection).
- Device registry, inventory/metrics ingestion, and a health evaluation
  engine (disk/CPU/RAM thresholds, sustained-condition detection, agent
  version checks).
- Append-only audit log (DB-trigger-enforced) covering auth, enrollment,
  device, and connectivity events.
- `wr-agent`: cross-platform (Linux + Windows, amd64/arm64) agent with
  Ed25519 device identity, OS-appropriate credential storage (Linux file
  mode 0600, Windows DPAPI machine scope), reconnect with exponential
  backoff+jitter, and gopsutil-based inventory/metrics collection.
- `wr-core createadmin` CLI for explicit, non-interactive super_admin
  bootstrap (no open registration).
- Windows Service / systemd integration for both `wr-agent` and `wr-core`,
  with install/uninstall scripts for both operating systems.
- Docker Compose deployment (internal-only Postgres, loopback-only admin
  web) and Dockerfile.
- Minimal admin web dashboard (React + TypeScript): login/TOTP setup,
  device list with health/status counts, device detail (overview,
  monitoring, audit tabs).
- Unit tests for password hashing, RBAC, rate limiting, protocol codec, and
  the health evaluation engine; database-backed integration tests for
  enrollment single-use and session validation.
- Remote terminal sessions: server-side session broker (`internal/remotesession`),
  an in-process relay (`internal/relay`) bridging binary stream frames between
  the browser WebSocket and the agent's control channel, Linux PTY
  (`creack/pty`) and Windows ConPTY (`UserExistsError/conpty`) providers, and
  an xterm.js terminal in the web dashboard.
- Temporary privilege elevation: reauth-gated `POST/DELETE
  /sessions/:id/privilege`, automatic expiry sweeper, countdown + revoke in
  the UI, per docs/SPECIFICATION.md §14.
- File transfer: list/mkdir/rename/delete via the existing `device_command`
  request/response path, and streamed download/upload over binary stream
  frames with agent-side atomic rename-on-finalize; web Files tab with
  browsing, upload, and download.
- Service management (`internal/platform/{linux,windows}`: systemd via
  `systemctl`, Windows via `golang.org/x/sys/windows/svc/mgr`) and process
  management (gopsutil, PID+start-time identity check before terminate),
  both exposed as device_command types with a web tab each.
- Customer management (`internal/customer`: customers + device groups) and
  automatic maintenance session tracking (`internal/maintenance`) tied to
  remote session open/close, with web Customers page, device→customer
  assignment, and a device Maintenance tab.
- SSH/RDP tunneling: single-use, SHA-256-hashed tunnel tickets
  (`internal/remotesession/tunnel.go`), a public (cookie-free)
  `/api/v1/tunnels/stream` endpoint authenticated solely by the ticket, and
  `wr-helper` — a native, loopback-only, single-connection Go binary that
  redeems a ticket and pipes a local TCP connection through the relay to
  the agent's `127.0.0.1:22`/`127.0.0.1:3389`. Web Remote tab gained a
  Tunnel panel that creates the ticket and shows the exact `wr-helper`
  command to run locally.
- Log access: `internal/platform/linux/logs_linux.go` (journalctl,
  `-o json`) and `internal/platform/windows/logs_windows.go` (`wevtutil qe
  System`), exposed as the `logs.query` device command with query/level/
  time-range/limit filters, `GET /api/v1/devices/:id/logs`, and a web Logs
  tab.
- Monitoring history: hourly-average downsampling (`device_metrics_hourly`,
  idempotent upsert rollup) and configurable raw/hourly retention pruning
  (`internal/device/retention.go`, per docs/DATABASE.md §3 and
  docs/CONFIGURATION.md `metrics.raw_retention`/`metrics.hourly_retention`),
  a `resolution=raw|hourly` param on `GET /devices/:id/metrics`, and a
  dependency-free inline-SVG chart component (`MetricsChart.tsx`) rendering
  CPU and RAM history in the web Monitoring tab.
- Signed agent updates (docs/AGENT.md §15): `cmd/wr-release-sign`, an
  offline Ed25519 signing tool (server and agent never hold a production
  private key); `internal/agentrelease` manages the `agent_versions`
  manifest and independently verifies each submission's signature against
  a server-configured trusted public key (`WR_RELEASE_PUBLIC_KEY_FILE`)
  before accepting it; `internal/agentupdate` implements
  hash+signature verification, atomic stage/backup/swap-while-running, and
  a marker-file crash-loop rollback (commits on the first successful
  post-update reconnect, restores the backup after
  `agentupdate.MaxBootAttempts` failed boots). The agent independently
  re-verifies every artifact against its own build-embedded trusted key
  (`-ldflags -X internal/agentcore.ReleasePublicKeyHex=...`) — a
  compromised server cannot push an unverifiable binary. New `agent.update`
  device command, `GET/POST /agent/releases`, `PATCH /agent/releases/:id`
  (block a bad release), `POST /devices/:id/update`; web Releases page and
  a "Check for update" control on the device detail page.
- Audit export: `GET /audit/export?format=json|csv` with the same
  user/device/event-type/time-range filters as the list endpoint,
  streamed as a downloadable attachment; the export action is itself
  audited (`audit.exported`). Web: export links on the device Audit tab.
- Structured rate limiting extended to the remaining public,
  unauthenticated attack surface: the control-channel handshake
  (`GET /agent/control`) and tunnel-ticket redemption
  (`GET /tunnels/stream`) are now per-source-IP rate limited, alongside
  the pre-existing login/MFA/reauth/enrollment limiters — forging a valid
  handshake or guessing a 256-bit ticket is already cryptographically
  infeasible, so this is defense-in-depth against connection-flood DoS
  and enumeration.
- Dashboard help: `internal/help` parses `docs/DASHBOARD_HELP.md`'s H2
  sections into slugged pages at startup, rendered through a small
  closed-subset Markdown→HTML converter that builds output from a fixed
  tag allowlist rather than stripping an open-ended input — every text
  run is HTML-escaped before any tag is wrapped around it, so this *is*
  the sanitization step, not a separate pass (unit-tested against
  embedded `<script>`/`onerror` payloads). `GET /help/index`,
  `GET /help/:slug`; web Help page with client-side title search.
- Secrets redaction: `auth.User` now has an explicit `MarshalJSON` that
  omits `PasswordHash`, so a future handler returning a `User` directly
  (instead of a hand-built redacted map) can't accidentally leak the
  Argon2 hash; a config test asserts the ephemeral dev-mode session
  pepper/TOTP key bytes never appear in log output, only the fact that
  they were generated.
- `scripts/create-db-runtime-role.sql` + docs/DEPLOYMENT.md §5a: creates a
  non-superuser `wartungsremote_app` Postgres role for wr-core's runtime
  connection (migrations still run under an owner/superuser role), scoped
  to DML on the app's own tables with explicit `REVOKE UPDATE, DELETE ON
  audit_log` alongside the existing DB-trigger enforcement.
- Recovery/revocation completeness (docs/SECURITY.md §20 Incident
  Response, docs/SPECIFICATION.md §23): bulk enrollment-token revocation
  (`POST /enrollments/revoke-all`); user account management — list users,
  set status active/disabled/locked (revoking all of that user's sessions
  immediately, not just blocking future logins), and a standalone
  "revoke all sessions" action, with a guard against an admin
  disabling/locking their own account; agent-version blocking enforced at
  the control-channel handshake itself (`controlhub.Hub`'s
  `VersionBlockedChecker`, wired to the existing `agent_versions.blocked`
  flag also used by Releases) rather than only as an update-manifest
  filter; and marking every still-active remote session `interrupted`
  (`close_reason: server_restart`) on server startup, since a restart
  drops every control-channel connection and no per-device disconnect
  event would otherwise fire for sessions that were active at that
  moment. Web: a Users page and the revoke-all-tokens control on the
  device list.
- Alerting rule engine (`internal/alerting`): user-configurable rules
  (offline, CPU/RAM/disk threshold, service-not-running, agent version)
  scoped to global/customer/group/device, evaluated on a 1-minute sweep
  against existing device/metrics state (and, for the service rule, a live
  `services.list` device command). Alerts open once and auto-resolve when
  the condition clears, both fully audited (`alert.opened`/`alert.resolved`
  with `reason: condition_cleared`); manual acknowledge/resolve via
  `POST /alerts/:id/{acknowledge,resolve}` is audited as user-actioned. New
  `alert.manage` permission gates rule/alert management; viewing reuses
  `monitoring.read`. Web: an Alerts page (rule builder, alert list with
  state filter) and an open-alert-count badge in the top nav.

### Fixed during Terminal/Files/Services/Processes verification

- Session-stream WebSocket (`/api/v1/sessions/:id/stream`) rejected every
  connection proxied through a different origin than the API's own Host
  header (coder/websocket's default Origin check) — broke both this
  project's own Vite dev proxy and any real reverse-proxy deployment where
  the SPA and API share a public origin but not the internal one wr-core
  sees. Fixed by explicitly allowing all origins at the WS layer, since the
  actual authorization boundary is the SameSite=Strict session cookie plus
  the permission check already performed before `Accept`.
- Several list endpoints (audit log, customers, groups, maintenance
  history) returned a bare Go `nil` slice when empty, which encodes as JSON
  `null` rather than `[]`; the dashboard called `.map()` on it directly and
  crashed. Fixed at the source (`out := []T{}` instead of `var out []T`).
- The agent's own `session_close` notification (e.g. when a terminal
  process exits by itself) had no handler on the server, so it was logged
  as a `protocol.error` audit failure on every session end. Added a
  `SessionCloseHandler` hook so agent-initiated closes update
  `remote_sessions` state instead of being treated as malformed input.
- The privilege-session expiry sweeper revoked expired privilege sessions
  but never audited it — only the explicit "revoke now" API path did,
  leaving automatic expiry (docs/SPECIFICATION.md §14 step 7) unaudited
  despite docs/SPECIFICATION.md §20 requiring "Privilege ... Ende" to be
  logged. Fixed by auditing the sweeper's revocations too.
- If the agent's control connection dropped while a terminal session was
  active, the session stayed `active` in the database and the browser's
  WebSocket simply hung until the session's own 30-minute expiry — nothing
  interrupted it on disconnect. Added a `DeviceDisconnectHandler` hook that
  marks every active remote session for that device `interrupted` and
  unblocks the browser side immediately, per docs/RELAY.md §8 and
  docs/STATE_MACHINES.md §3. Verified by killing the agent process mid-session.
- Closing a terminal both via the "Close Session" button and via the
  browser WebSocket disconnecting independently double-ran the close path
  (duplicate audit entries, redundant agent notification). Made both close
  paths idempotent by re-checking session state before acting.

### Live-verified during Recovery/Revocation testing

Against the actual running demo server, database, and agent: opened a real
terminal session then restarted wr-core, confirming the session flipped to
`interrupted`/`server_restart` and was audited. Created two enrollment
tokens and bulk-revoked them, confirming both got `revoked_at` set.
Created a second user, logged them in, then (as the first admin) disabled
them, locked them, and revoked their sessions individually — each action
verified against the live session (a revoked/locked user's existing
session immediately started returning 401, and a locked user's login
attempt was rejected). Confirmed an admin cannot disable/lock their own
account. Most notably: inserted a manifest row marking the live agent's
own exact (version, os, arch) as blocked, restarted the real running
agent, and watched the server reject its handshake
(`StatusPolicyViolation "authentication failed"`) on every reconnect
attempt — then removed the block and confirmed it reconnected normally
again.

### Live-verified during Agent Update testing

Full pipeline exercised against the actual running demo agent (not a
throwaway copy): a signed test release pointed at a byte-identical rebuild
of the live `wr-agent.exe` (served from a temporary local HTTP file
server), triggered via the real `POST /devices/:id/update` API. Confirmed:
server-side signature verification accepts a validly-signed release and
rejects a tampered one; the agent downloaded, independently re-verified,
staged, and atomically swapped its own running executable (`os.Rename` on
a currently-executing image — the file hashes matched byte-for-byte
afterward); the process exited cleanly; restarting it (standing in for a
service manager's auto-restart) reconnected and committed the update
(marker + backup binary deleted); a second trigger with the artifact
server stopped failed cleanly during download with no partial swap and no
agent crash. The rollback/crash-loop path itself (`MaxBootAttempts`
exceeded → `RestoreBackup`) was intentionally NOT exercised against the
live agent — deliberately crash-looping the demo agent to test it wasn't
worth the risk of leaving it in a broken state — but its primitives
(`StageAndSwap`, `RestoreBackup`, the marker boot-attempt counter) are
covered by `internal/agentupdate`'s unit tests.

### Known limitations found during Audit Export / Production Hardening work

- `scripts/create-db-runtime-role.sql` could not be executed end-to-end
  against the dev database: this session's Postgres superuser role has no
  password set (the cluster was initialized with `--auth=trust` for local
  setup, then switched to `scram-sha-256`, and setting a password
  retroactively requires either an already-authenticated superuser
  connection or reopening trust auth — both circular or a security-config
  change out of scope for testing a deployment script). Every statement
  in the script *did* reach PostgreSQL and returned semantic errors
  ("permission denied to create role", then "role does not exist" for
  every dependent statement) rather than syntax errors, which confirms
  the SQL parses correctly; full execution (actually creating the role
  and confirming its grants) needs a real superuser connection, which a
  production deployment's admin DSN provides but this sandboxed dev
  cluster intentionally doesn't.

### Known limitations found during Alerting verification

- The "service" alert rule type calls the same `services.list` device
  command the Services tab uses. In this dev environment the agent runs as
  an unprivileged foreground process rather than an installed Windows
  service (which normally runs as `LocalSystem`), so `OpenSCManager` fails
  with access denied and the rule silently never triggers (by design — an
  agent-side command error must not cause a false alert). The rule engine
  itself was live-verified for offline/cpu/ram/disk/agent_version (open,
  acknowledge, auto-resolve, full audit trail); the service rule's
  positive-trigger path is implemented and code-reviewed but not
  live-exercised here — it needs the agent installed as a service
  (`deployment/windows/install-agent.ps1`) to get SCM access.

### Fixed during Tunnel/Logs verification

- Both `handleSessionStream` (terminal) and `handleTunnelStream` (tunnel)
  shared a single `context.Context` between the read loop and the
  forward-to-browser goroutine; nothing cancelled it when the read side
  exited first (e.g. the browser or `wr-helper` closing its end), so the
  writer goroutine blocked forever on an empty channel and cleanup
  (`broker.Unregister`, DB close, audit) never ran — a session or tunnel
  would stay `active` indefinitely. Fixed by deriving a cancellable
  `context.Context` from the deadline context and cancelling it as soon as
  the read loop exits. Verified live: after this fix, `tunnel_sessions`
  correctly transitions to `closed` with accurate byte counters and the
  full `tunnel.opened → tunnel.connected → tunnel.closed` audit trail.

### Fixed during initial verification

- Session token hash was computed from raw random bytes at creation but
  from the encoded token string at validation, so no session ever
  validated after login. Fixed by hashing the same encoded string in both
  places, and covered by an integration test.
- `client_ip` values (`host:port` from `net/http`) were passed directly into
  Postgres `inet` columns, breaking session/audit inserts. Fixed by
  stripping the port (and taking the first hop of `X-Forwarded-For`).
- `user_roles` used an expression (`COALESCE(...)`) directly inside a
  `PRIMARY KEY` constraint, which PostgreSQL rejects; replaced with a
  surrogate key plus an expression-based unique index.
- `mode: development` left TOTP/session secrets empty, breaking MFA setup
  even in local dev; development mode now generates ephemeral secrets with
  a logged warning, while production mode still refuses to start without
  real secret files.

### Fixed during final verification pass

- `InterruptAllForDevice` (agent disconnect) and `InterruptAllActive`
  (server restart) marked the `remote_sessions` row `interrupted` but
  never closed the linked `maintenance_sessions` row, leaving it stuck
  "in progress" forever — a pre-existing gap (present even before this
  session's `InterruptAllActive` was added, since `InterruptAllForDevice`
  had the same bug) caught by live-testing the Maintenance tab after a
  restart. Fixed by having both repo methods return the linked
  `maintenance_session_id` alongside each interrupted session id, and
  closing it (`result: "interrupted"`) at both call sites in
  `internal/httpapi/router.go`. Verified live: opened a terminal session,
  restarted wr-core, and confirmed the Maintenance tab shows the record
  closed with a result and end time instead of stuck open.

### Not yet implemented

Disk/network monitoring history charts, alert notification channels
(email/ntfy/Telegram/webhooks/ioBroker — the rule engine itself is
implemented), an audit hash-chain, and context links from dashboard error
messages to help pages are intentionally deferred — see `docs/TODO.md` for
the phase-by-phase plan.
