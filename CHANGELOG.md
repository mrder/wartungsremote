# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added: HTTPS/security configuration advisories (agent v0.2.1)

- wr-core deliberately never terminates TLS itself (a reverse proxy is
  expected to) — but a `public.base_url` that isn't `https://` is a
  reliable signal nobody has set that up yet. Now surfaced two ways
  rather than silently ignored: logged as a startup warning, and shown
  as a dashboard banner to anyone with `system.settings` (via a new
  `advisories` field on `GET /me`). Also flags plain `mode: development`
  as an informational reminder.
- Complements this with an honest per-device signal: every agent now
  reports at handshake whether it actually dialed `wss://` (i.e. its own
  `server_url` is `https://`), stored as `devices.transport_secure` and
  shown as an "Unencrypted" badge on that device's Overview tab. Catches
  the case where a correctly-configured HTTPS server still has one
  leftover agent pointed at an old `http://` URL, which a server-side-only
  check could never see.
- Agent itself also logs a local startup warning if its own `server_url`
  isn't HTTPS, independent of anything server-side.
- Live-verified: built and signed a real v0.2.1 release, and for the
  first time actually exercised the self-update mechanism end-to-end for
  real (not just its signature-verification code in isolation) —
  download, verify, atomic swap, restart, "update committed" on
  reconnect — since every previously-built dev agent predated having the
  release-signing key embedded and could never attempt it. Confirmed the
  new banners and per-device badge with real data afterward.

### Added: network traffic history (agent v0.2.0)

- New per-device network traffic charts (sent/received, both total
  system traffic and this agent's own control-channel overhead
  separately) and a cross-device "Network usage" ranking page — "which
  client is using how much bandwidth."
- Unlike CPU/RAM/disk (pushed live every status interval), traffic is
  sampled locally on the agent roughly once a minute, buffered in a new
  local SQLite file (`internal/netmetrics`, pure-Go `modernc.org/sqlite`
  so the existing one-line cross-compile keeps working unchanged — no
  cgo, no per-platform C toolchain needed), and uploaded to the server in
  batches every 5 minutes (configurable, `agent.network_upload_interval`)
  — plus immediately on every reconnect, so a period offline delays
  delivery rather than losing the data. New `network_metrics_batch`
  protocol message; no protocol version bump needed since an older agent
  simply never sends one.
- New `device_network_metrics`/`device_network_metrics_hourly` tables
  (separate from `device_metrics` — different sampling cadence, and
  hourly rollup is a SUM of bytes transferred that hour rather than an
  average, which is the more meaningful figure for a volume metric).
  Dashboard-adjustable retention (Settings → Network traffic history
  retention), shorter raw default (7 days) than CPU/RAM/disk given the
  higher sample rate.
- Requires updating installed agents to v0.2.0+ to actually start
  collecting this — older agents keep working exactly as before, they
  just won't have network charts until updated.

### Fixed: concurrent startups could race on database migrations

- `internal/db.Migrate` checked "is this migration applied yet?" and
  applied it as two separate, unsynchronized steps — fine for the
  overwhelmingly common case of one process migrating an empty or
  up-to-date database, but if two processes ever start against the same
  database at once (an overlapping rolling restart; also just two test
  binaries sharing one database, which is how this was actually found),
  both see "not applied" and both run it, and anything with a
  globally-unique name (`CREATE EXTENSION`, `CREATE TYPE`) fails outright
  instead of harmlessly re-running. Fixed with a `pg_advisory_lock` held
  for the whole migration pass, so a second caller just waits its turn
  and then finds everything already applied.
- Live-verified by deliberately reproducing the race (two test packages
  pointed at the same fresh database) before the fix — reliably failed —
  and confirming three clean runs in a row after it.

### Added: audit log hash chain, and a way to verify it

- `audit_log.prev_hash`/`entry_hash` columns have existed in the schema
  since the very first migration but were never actually populated —
  found while looking for something worth adding here. Every audit entry
  is now chained to the one before it (`entry_hash = SHA256(prev_hash ||
  this entry's fields)`), computed in Go rather than a DB trigger so the
  write path and the verify path can never quietly drift apart on
  serialization format. Concurrent inserts are serialized with
  `pg_advisory_xact_lock` so the chain can't fork under load.
- Added `POST /api/v1/audit/verify` (permission `audit.read`) and a
  "Verify chain" button under Settings → Audit log integrity: recomputes
  every entry's hash from scratch and reports whether anything since the
  chain started has been altered. Combined with the existing append-only
  DB trigger, an edit made directly against the database — bypassing the
  application entirely — is now detectable, not just blocked.
- Rows inserted before this shipped have no hash to check; a leading run
  of those is correctly reported as "pre-chain" rather than as tampering
  — this only matters on an install with existing audit history (i.e.
  every real one), and was caught by testing against the actual
  months-old local dev database rather than a fresh one.
- Found and fixed two real bugs live-testing this against that database:
  (1) the hash was computed from a nanosecond-precision timestamp before
  it was known that Postgres' `timestamptz` only keeps microseconds,
  silently truncating the value it later reads back and breaking every
  single entry's verification; (2) the driver reads timestamps back in
  the local timezone rather than UTC, so the same instant formatted with
  a different offset and, again, broke every verification. Both are
  now normalized before hashing on both the write and read side.
- Live-verified end to end against the real dev server and its 29
  pre-existing audit entries: logged in through the actual dashboard,
  clicked "Verify chain", got back "Chain intact — 4 entries checked, no
  tampering detected. (29 older entries predate this feature...)" —
  correct on both counts.

### Added: reusable enrollment tokens, and a way to actually see/revoke them

- Bulk rollouts (many devices, one token) were previously not possible —
  every enrollment token was single-use only. Added `reusable: true` on
  token creation: installs any number of devices until it expires or is
  revoked, with a much longer default/max validity (90 days vs. 24h for
  single-use) since it's meant for a staged rollout, not a one-off
  install. Deliberate accepted risk: anyone holding the token can enroll
  additional devices for as long as it's valid, but that's the full
  extent of it — enrollment alone grants no access beyond "a device
  shows up in the list" (docs/AGENT.md §5).
- Found while building this: `GET /enrollments` and `DELETE
  /enrollments/:id` were already normatively specified (docs/API.md §4)
  but `GET` was never implemented — meaning individual token revocation
  was already wired up server-side this whole time but completely
  unreachable from the dashboard (no way to discover a token's ID).
  Added the list endpoint and a "show outstanding tokens" panel with
  per-token revoke, so it's no longer all-or-nothing (revoke-all).
- Live-verified end to end: created a reusable token, enrolled two
  different devices with it, confirmed the outstanding list showed
  `use_count: 2`, revoked it individually, confirmed a third enrollment
  attempt was correctly rejected afterward.

### Added: automatic least-privilege database role for Docker Compose

- `docs/DEPLOYMENT.md` §5a has long documented running wr-core against a
  restricted `wartungsremote_app` role (no DDL rights) instead of the
  migration-owner role, but it was a fully manual, easy-to-skip step.
  wr-core now supports two separate DSNs — `WR_MIGRATION_DATABASE_URL_FILE`
  (DDL rights, used only for the startup migration step) and
  `WR_DATABASE_URL_FILE` (the runtime connection, everything else) —
  falling back to one DSN for both when the migration one isn't set, so
  every existing native/dev install keeps working unchanged.
- Docker Compose now sets this up automatically: a new `db-init` service
  creates/updates the restricted role on every startup (idempotent —
  `scripts/ensure-db-runtime-role.sql`) before wr-core starts, which is
  wired to the two DSNs out of the box. Nothing to configure beyond the
  existing `generate-docker-secrets.sh` step.
- Found and fixed while testing the new SQL script: psql's `:variable`
  substitution doesn't happen inside dollar-quoted (`DO $$ ... $$`)
  blocks — the password had to be set via a plain `ALTER ROLE` statement
  immediately after, not inside the conditional-create block itself.
- Live-verified: ran the real migration, watched it apply cleanly via the
  owner role, then confirmed the server (including a real login) runs
  entirely on the restricted role with zero DDL rights.

### Verified: backup/restore actually works end to end

- Ran the real mechanism scripts/backup-server.sh uses (pg_dump, gzip,
  AES-256-CBC/pbkdf2 encrypt+decrypt) against the live local dev database,
  restored the dump into a brand-new database, and confirmed every
  table's row count matched exactly. Then went further: started a second
  wr-core instance pointed at the *restored* database and logged in with
  the original admin password — succeeded, proving the restored data
  (including the Argon2id hash) is genuinely usable, not just
  structurally similar. The docker-compose-specific wrapper (`docker
  compose exec postgres pg_dump`) itself still needs a real Docker
  daemon to verify, same caveat as the rest of the Compose deployment.

### Added: dedicated remote-support OS account for the SSH/RDP tunnel

- Real gap found by re-examining the SSH/RDP tunnel feature: it only
  forwards raw network traffic to the device's own existing SSH/RDP
  service — a login completely separate from our Ed25519 device identity.
  Without a known account on the target, the tunnel needs the customer's
  own credentials, defeating the "remote support without needing the
  customer's login" goal that already holds for Terminal/Files/Services.
- The agent now provisions a dedicated local account ("remotewartung",
  never the customer's own root/Administrator account) once, on first
  successful connection: `useradd`+`chpasswd` and a best-effort
  `sudoers.d` drop-in on Linux (sudo group naming varies by distro, and
  some minimal distros don't ship sudo at all — SSH login still works
  either way, Terminal already has unconditional root regardless), local
  Administrator via `net user`/`net localgroup` on Windows.
- The generated password is reported once over the already-authenticated
  control channel (new `support_credential_report` message) and stored
  AES-256-GCM-encrypted (`internal/support`, same key as TOTP secrets) —
  plaintext only ever exists in an explicit, audited dashboard reveal.
  Rotatable on demand from the device's Remote tab; rotation re-runs the
  same provisioning logic and reports the new password the same way.
- New migration `0007_support_credential.sql`. Cross-compiles clean for
  both `GOOS=linux` and `GOOS=windows`; encryption round-trip covered by
  a unit test. Not yet live-verified against a real device — the actual
  account creation should be tested deliberately (it's a persistent
  change to a real machine), not exercised automatically.

### Fix: native Linux installers assumed a writable /usr/local/bin

- Found live installing on a real ZimaOS box: `install(1)` doesn't create
  missing parent directories, and some distros (ZimaOS among them) ship a
  read-only root filesystem where `/usr/local` isn't writable at all —
  only specific persistent-state paths are. Moved the installed binary
  for both `install-agent-linux.sh` (→ `/var/lib/wartungsremote/wr-agent`)
  and `install-core-linux.sh` (→ `/var/lib/wartungsremote-core/wr-core`),
  updated both systemd units' `ExecStart` and both uninstall scripts to
  match. Native Windows installs are unaffected.

### Added: disk history charts, dashboard-adjustable retention, alert deletion

- Disk usage was already collected in every metrics sample
  (`device_metrics.filesystems`) but never aggregated or charted. Added
  `disk_used_bytes`/`disk_total_bytes` (summed across non-removable
  filesystems, same treatment as health thresholds) to both the raw and
  hourly-rollup tables, and a Disk chart next to CPU/RAM on the device
  Monitoring tab. Network history is explicitly NOT included — the agent
  doesn't collect network throughput at all currently, so that needs an
  agent-side change and a new signed release, not just a chart.
- Metrics retention (`raw_retention`/`hourly_retention`) was previously
  only a static `server.yaml` value read once at startup. Added
  `internal/appsettings` (a small DB-backed key/value store) and a new
  Settings page (permission: `system.settings`, already seeded for
  super_admin but never wired to anything until now) so retention can be
  changed from the dashboard and takes effect on the next sweep tick, no
  restart needed.
- Alerts could be acknowledged/resolved but never deleted — they're kept
  indefinitely by design (no retention sweep), so acknowledge/resolve was
  the only way to make one go away permanently. Added `DELETE
  /api/v1/alerts/{id}` (audited) and a Delete button.
- Confirmed while investigating: switching a device between the beta and
  stable channel already worked with no code change needed — there was
  never a version-ordering/anti-rollback check anywhere, only signature
  verification, so picking "stable" after a bad beta build already just
  installs whatever's currently tagged stable, newer or not.
- New migration `0006_disk_metrics_and_settings.sql`. Code builds/tests/
  typechecks clean but this batch has NOT been live-verified end to end
  (local dev Postgres was unreachable when this was built) — verify
  against a real running stack before relying on it.

### Added: configurable backup cron, CI, license

- `scripts/backup-server.sh` + `scripts/install-backup-cron.sh`: dumps the
  Docker Compose Postgres DB plus config/secrets into one archive,
  optional AES-256 encryption, configurable retention (prunes old
  archives automatically) and cron schedule — nothing hardcoded, all
  flags/env vars. Re-running the installer replaces its own crontab line
  instead of duplicating it.
- `.github/workflows/ci.yml`: build/vet/unit-tests/integration-tests (Go)
  and typecheck/build (web) on every push/PR, plus a `docker compose
  config` validation step. Deliberately does NOT sign or publish agent
  releases — the signing key stays offline per the existing design
  (README "Signing agent releases"); CI never sees it.
- Added `LICENSE` (source-available: free to run your own instance,
  commercial hosting/resale needs separate permission — not an
  OSI-approved open source license) and a "Powered by sonnyathome.online"
  footer on every dashboard page and in the README.

### Docker Compose: the dashboard itself was never actually servable

- `docs/DEPLOYMENT.md`'s own recommended layout lists a `wr-web`
  component, but `docker-compose.yml` never had one — wr-core is a pure
  JSON API with no static-file serving of its own, so after `docker
  compose up` there was genuinely no way to open the admin dashboard
  against a Dockerized server at all.
- Added a `wr-web` service: a small multi-stage build (`web/Dockerfile`)
  that builds the React app and serves it via Caddy
  (`deployment/docker/Caddyfile.web`), proxying `/api/*` to `wr-core` on
  the internal Docker network. Bound to the host's `127.0.0.1` only, same
  "SSH tunnel or VPN, never public" rule as everything admin-facing.
- Found and fixed a real bug surfaced while wiring this up:
  `server.example.yaml` had `admin.listen: 127.0.0.1`, which — inside a
  container — binds the container's own loopback, unreachable by any
  other container (including the new wr-web) or by Docker's own
  published-port forwarding. Changed to `0.0.0.0` for the Docker
  reference config specifically, with the actual "never public" boundary
  now correctly enforced by the internal Docker network plus the
  loopback-bound host port mapping instead of the bind address. The
  native (non-Docker) `ServerConfig.Default()` keeps the literal
  `127.0.0.1` default, which is correct there.
- The Docker Compose path also had no documented way to create the first
  admin account at all. `generate-docker-secrets.sh`/`.ps1` now also
  generate an `admin_password.txt`, wired as a compose secret purely so
  `docker compose run --rm wr-core createadmin --username admin
  --password-file /run/secrets/admin_password` (now documented in the
  README) has something to read.

### Docker Compose reference deployment: TLS/domain was actually missing

- Found while preparing the Docker+GitHub deployment path: the shipped
  `deployment/docker/docker-compose.yml` never had anything doing TLS
  termination — `docs/DEPLOYMENT.md` §4 documented reverse-proxy/TLS
  requirements, but no such component existed in the stack itself, so
  `docker compose up` alone could never make a real domain reachable
  over HTTPS.
- Added a `caddy` service (`deployment/docker/Caddyfile`) fronting only
  the agent gateway — automatic Let's Encrypt cert for `WR_DOMAIN`
  (`.env`, see new `.env.example`), zero manual certificate handling,
  WebSocket upgrade works out of the box via `reverse_proxy`, proxy
  timeouts disabled for the long-lived control-channel connections. The
  admin web stays loopback-only and is never routed through Caddy.
- Gave the `public` Docker network a fixed subnet so the new
  `security.trusted_proxies` (see above) can name Caddy precisely in
  `server.example.yaml` — without it, X-Forwarded-For from Caddy itself
  would otherwise be correctly-but-unhelpfully ignored post-fix.
- Also found: `docker-compose.yml` mounted `server.example.yaml`
  straight into the container as the live config, and never wired
  `WR_RELEASE_PUBLIC_KEY_FILE` at all — meaning publishing any signed
  agent release through the dashboard would always fail with an empty
  trust key in this reference deployment. Fixed by mounting a real
  `server.yaml` (copied from the example, gitignored) instead, and by
  wiring `keys/release.pub` in as the `release_public_key` secret.

### Security fix: X-Forwarded-For was trusted unconditionally

- `clientIP()` (used for every device IP-history entry and every
  `SourceIP` recorded in the audit log) honored `X-Forwarded-For`
  regardless of who sent it — meaning any agent or caller reaching
  `public.listen` could set that header to make the server log an
  arbitrary IP on its behalf, undermining the audit trail. Found while
  investigating a question about how the public IP shown in the
  dashboard behaves once wr-core runs behind Docker/a reverse proxy.
- Added `internal/netutil` with a `TrustedProxies` allowlist and a new
  `security.trusted_proxies` config option (default: empty, i.e. nobody
  trusted — `X-Forwarded-For` is always ignored and the raw TCP peer
  address is used). Only needs to be set to the exact address of an
  actual reverse proxy in front of wr-core; a plain Docker port mapping
  needs nothing, since Docker already preserves the real client IP.
  `internal/controlhub.Hub` and `internal/httpapi`'s `handlers` now both
  resolve client IPs through this shared, trusted-proxy-aware helper
  instead of two separately duplicated, unconditionally-trusting copies.
- Live-verified against the running dev server: a forged
  `X-Forwarded-For: 6.6.6.6` on an enrollment request was correctly
  ignored post-fix (audit log recorded the real `127.0.0.1` peer
  address instead).

### Security-relevant decision: Linux agent now runs as root

- Changed `scripts/install-agent-linux.sh` and
  `deployment/systemd/wartungsremote-agent.service` from the
  V1-documented unprivileged dedicated service account
  (`User=wartungsremote`, `NoNewPrivileges`, `ProtectSystem=strict`,
  `ProtectHome`) to running as **root**. Deliberate product decision, not
  a bug: without it, Terminal/Services/Processes/Files on Linux only ever
  had the unprivileged account's rights, so genuine remote admin support
  would have needed the customer's own sudo password — defeating the
  point. Windows already reached full capability via the Windows Service
  running as LocalSystem; this makes Linux match.
- Accepted tradeoff, discussed explicitly before changing: a compromised
  wr-core server, or a bug in the agent's own command handling, is now
  immediately root on every enrolled Linux device — considered and
  rejected the smaller-blast-radius alternative (a sudoers policy scoped
  to only the actions actually needed). The required compensating
  control is network isolation of the admin dashboard, which already
  existed by default (`admin.listen: 127.0.0.1:9443`,
  `docker-compose.yml` publishing 9443 on loopback only) — only the
  agent-facing gateway is ever internet-reachable, and it has no login to
  attack. See README.md "Security" and `docs/DEPLOYMENT.md` §2-3.
- The temporary "request admin rights" UI/audit flow (`docs/SPECIFICATION.md`
  §14) still only does audit bookkeeping — it does not (and, with the
  agent already running as root, no longer needs to) actually vary the
  OS-level privilege of an open session.

### Published

- Initial public repository: https://github.com/mrder/wartungsremote.
  First signed release `v0.1.1` (Windows amd64, Linux amd64/arm64),
  signed offline with `cmd/wr-release-sign` and registered in the agent
  manifest with real `github.com/.../releases/download/...` artifact
  URLs. End-to-end self-update verified against this real release: the
  live agent downloaded, verified, staged, and swapped itself from
  `0.1.0-dev` to `0.1.1`, byte-identical to the uploaded asset.

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
