# WartungsRemote

WartungsRemote is a transparent, authorized remote maintenance platform for
Windows and Linux devices. Devices always initiate an outbound connection to
the server — no inbound router port-forwarding is required on the customer
side. Existing SSH (22) and RDP (3389) services are never modified or
occupied by WartungsRemote.

The full normative specification lives in [`docs/`](docs/README.md). Start
there — this file is a practical build/run guide for the implementation in
this repository, not a replacement for the spec.

## Status

This repository implements and has live-verified, against a real PostgreSQL
database and real Windows/Linux agent builds:

```
Server starts
  -> Admin login + TOTP
  -> Enrollment token creation
  -> Agent enrollment (Linux + Windows)
  -> Authenticated control channel (Ed25519 challenge/response)
  -> Heartbeat, online/offline state machine
  -> Inventory + live metrics
  -> Health evaluation engine
  -> Temporary privilege elevation (reauth-gated, auto-expiring)
  -> Remote terminal (Linux PTY / Windows ConPTY) over a browser WebSocket
  -> SSH/RDP tunneling via the native `wr-helper` client (single-use ticket,
     loopback-only local port)
  -> File browsing, upload, download, mkdir, rename, delete
  -> Service management (systemd / Windows SCM)
  -> Process listing and termination
  -> Log access (journalctl / Windows Event Log) with query/level/time filters
  -> Monitoring history: hourly downsampling, dashboard-adjustable retention
     (Settings page, takes effect without a restart), CPU/RAM/disk charts
  -> Alerting: configurable rules (offline/CPU/RAM/disk/service/agent version),
     scoped to global/customer/group/device, with acknowledge/resolve and audit
  -> Signed agent self-updates: offline Ed25519 release signing, server- and
     agent-side signature verification, atomic swap-while-running, crash-loop
     rollback
  -> Incident response: bulk enrollment-token revocation, user
     disable/lock/all-sessions-revoke, agent-version blocking enforced at the
     control-channel handshake, sessions marked interrupted on server restart
  -> Audit export (JSON/CSV), dashboard help (rendered + sanitized from
     docs/DASHBOARD_HELP.md), rate limiting on the remaining public endpoints
  -> Customer management + automatic maintenance history
  -> Web dashboard (device list/detail, all of the above, audit)
  -> Audit log (append-only)
```

Terminal/tunnel/file/service/process sessions are also cleaned up correctly
when the agent disconnects mid-session (state -> `interrupted`, browser
socket closed) rather than hanging until their own expiry.

Disk history charts are code-complete and build/typecheck-verified, but not
yet live-verified end to end (the local dev database was unreachable when
this was built — re-verify against a real running stack before relying on
it). Not yet implemented at all: network monitoring history charts — the
agent doesn't currently collect network throughput data on the wire, so
this needs an agent-side change (both platforms) and a new signed release
before any data would even start flowing, not just a chart. Also not yet
implemented: alert notification channels (email/ntfy/Telegram/webhooks/
ioBroker), an audit hash-chain, and context links from dashboard error
messages to help pages — see `docs/TODO.md` for the full phase-by-phase
plan. `docs/TODO.md` is only checked off for items genuinely complete
(code + tests/live verification + audit + docs), not partially started
ones.

## Repository layout

```
cmd/wr-agent/       agent entrypoint (Windows Service / systemd)
cmd/wr-core/         server entrypoint (auth, device registry, control channel, HTTP API)
cmd/wr-helper/       native SSH/RDP tunnel helper (loopback-only, single-use ticket)
internal/            server + agent implementation packages
migrations/          embedded SQL migrations (PostgreSQL)
web/                 admin dashboard (React + TypeScript + Vite)
deployment/          Docker Compose, systemd units, Windows service scripts
scripts/             Linux/Windows install & secret-generation scripts
tests/               database-backed integration tests
docs/                normative specification (read this first)
```

## Prerequisites

- Go (matching `go.mod`)
- Node.js + npm (for `web/`)
- PostgreSQL 16+ (local install, or via Docker Compose)

## Installation order: the server always comes first

The **server (wr-core)** is the only thing you build/deploy up front. It's
what issues the one-time enrollment tokens and per-device install commands
that every **client (wr-agent)** needs — there is no way to install a
client before a server exists to enroll it against. Concretely:

1. Stand up wr-core (see "Quick start" below for a dev instance, or
   "Docker Compose deployment" / "Building and installing the server" for
   something longer-lived), reachable from the machines you want to
   support.
2. Log into its dashboard and set the account up (TOTP, etc.).
3. Click **"+ Add Device"**. The panel generates a ready-to-paste
   one-line install command — with your server's address and a fresh,
   single-use token already filled in — for both Linux and Windows,
   using the latest signed release from
   [GitHub Releases](https://github.com/mrder/wartungsremote/releases)
   (`agent-*` tags). No manual download/build step needed:

   ```bash
   # Linux — generated for you, shown once
   curl -fsSL https://raw.githubusercontent.com/mrder/wartungsremote/main/scripts/quickinstall-agent-linux.sh \
     | sudo bash -s -- --server-url https://remote.example.de --token wr_enroll_XXXX
   ```

4. Paste that one line into an SSH/PowerShell session on the target
   device. It downloads the correct signed binary for that OS/arch,
   verifies it, and installs it as a system service — done.

Each token is single-use and time-limited (docs/AGENT.md §5); generate a
new one per device from "+ Add Device". Building the client from source
instead of using a signed release is also supported — see "Building and
installing the agent" below — but the release + generated command is the
path meant for actually rolling this out, not just for local development.

## Quick start (local development)

1. **Database.** Point `WR_DATABASE_URL_DEV` at a throwaway Postgres
   database. Migrations run automatically on `wr-core` startup.

   ```bash
   export WR_DATABASE_URL_DEV="postgres://user:pass@127.0.0.1:5432/wartungsremote?sslmode=disable"
   ```

2. **Server config.** Copy the example and adjust as needed:

   ```bash
   cp deployment/docker/server.example.yaml server.yaml
   ```

   For local testing you can set `mode: development` in `server.yaml`. In
   development mode, if `WR_SESSION_PEPPER_FILE` / `WR_TOTP_ENCRYPTION_KEY_FILE`
   are not set, wr-core generates **ephemeral** secrets at startup (logged as
   a warning) — sessions and TOTP enrollments will not survive a restart.
   For anything beyond a quick smoke test, set real secret files:

   ```bash
   head -c 32 /dev/urandom > .devsecrets/session_pepper.bin
   head -c 32 /dev/urandom > .devsecrets/totp_key.bin
   export WR_SESSION_PEPPER_FILE="$PWD/.devsecrets/session_pepper.bin"
   export WR_TOTP_ENCRYPTION_KEY_FILE="$PWD/.devsecrets/totp_key.bin"
   ```

   `mode: production` (the default) additionally requires
   `WR_DATABASE_URL_FILE` and refuses to start with weak/missing
   configuration — see `docs/CONFIGURATION.md`.

3. **Build and run the server:**

   ```bash
   go build -o wr-core ./cmd/wr-core      # wr-core.exe on Windows
   ./wr-core
   ```

4. **Create the first super_admin** (no open registration, per
   `docs/SPECIFICATION.md` §12):

   ```bash
   echo -n "a-strong-passphrase-12+chars" > /tmp/admin_pw.txt
   ./wr-core createadmin --username admin --password-file /tmp/admin_pw.txt
   ```

5. **Web dashboard:**

   ```bash
   cd web
   npm install
   npm run dev
   ```

   Open the printed URL, log in, and complete TOTP setup (scan the QR code
   shown on first login).

6. **Enroll an agent.** In the dashboard, use "+ Add Device" to create a
   one-time enrollment token, then install the agent (see below) with that
   token.

## Building and installing the agent

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o wr-agent ./cmd/wr-agent
sudo ./scripts/install-agent-linux.sh --server-url https://remote.example.de --token wr_enroll_XXXX

# Windows (run install-agent.ps1 as Administrator)
go build -o wr-agent.exe .\cmd\wr-agent
.\deployment\windows\install-agent.ps1 -ServerUrl "https://remote.example.de" -Token "wr_enroll_XXXX"
```

Both installers set up the documented config/data/log directories from
`docs/AGENT.md` §2, store the enrollment token as a one-time file (deleted
after successful enrollment), and register the agent as a proper system
service — visibly, with no hidden functionality (`docs/PROJECT_CONCEPT.md`
§36).

## Building and installing the server (native, non-Docker)

For a containerized deployment, use `deployment/docker/docker-compose.yml`
(see `docs/DEPLOYMENT.md`). To run wr-core natively as a service on either
OS instead:

```bash
# Linux
go build -o wr-core ./cmd/wr-core
sudo ./scripts/install-core-linux.sh --database-url "postgres://wruser:pass@localhost:5432/wartungsremote"

# Windows (run install-core.ps1 as Administrator)
go build -o wr-core.exe .\cmd\wr-core
.\deployment\windows\install-core.ps1 -DatabaseUrl "postgres://wruser:pass@localhost:5432/wartungsremote"
```

## Signing agent releases

Release artifacts are signed **offline**, never by wr-core or wr-agent
themselves (see docs/AGENT.md §15):

```bash
# Once, offline, on a machine that never touches the server:
go build -o wr-release-sign ./cmd/wr-release-sign
./wr-release-sign -genkey -out release
# -> release.key (PRIVATE — keep offline), release.pub (install below)

# Rebuild the agent with the trusted public key embedded:
go build -ldflags "-X wartungsremote/internal/agentcore.ReleasePublicKeyHex=$(xxd -p release.pub | tr -d '\n')" -o wr-agent ./cmd/wr-agent

# Point the server at the same public key (raw 32 bytes):
export WR_RELEASE_PUBLIC_KEY_FILE=/path/to/release.pub

# For each new release artifact:
./wr-release-sign -sign wr-agent-v0.2.0 -key release.key
# -> prints artifact_sha256 and signature; POST them to /api/v1/agent/releases
```

## Docker Compose deployment

This is the recommended way to run the server for real use (see
"Installation order" above). It brings its own reverse proxy (Caddy) that
automatically gets and renews a Let's Encrypt TLS certificate for your
domain — no manual certificate handling needed, just DNS pointed at the
server:

```bash
cd deployment/docker
cp .env.example .env
# edit .env: set WR_DOMAIN to the domain you've pointed at this server
cp server.example.yaml server.yaml
# edit server.yaml: public.base_url must be https://<the same WR_DOMAIN>
../../scripts/generate-docker-secrets.sh      # or .ps1 on Windows
docker compose up -d
```

Create the first super_admin (reads the password generated above, one-off,
non-interactive — the same rule as native installs, no open registration):

```bash
docker compose run --rm wr-core createadmin --username admin --password-file /run/secrets/admin_password
cat secrets/admin_password.txt   # the password to log in with — change it after first login
```

The dashboard (`wr-web`) and the admin API port are both bound to the
host's `127.0.0.1` only by design — reach them via SSH tunnel or your
management VPN, never expose either publicly (`docs/DEPLOYMENT.md` §2-3,
§10):

```bash
ssh -L 9080:127.0.0.1:9080 you@your-server
# then open http://127.0.0.1:9080 in your local browser (must be opened as
# 127.0.0.1/localhost specifically, not the server's LAN/public IP, for
# the session cookie to work without TLS on this loopback-only hop)
```

Only the agent gateway is fronted by Caddy on 443 — that's the one thing
meant to be reachable from the internet.

Set up automatic backups (database + config + secrets, encrypted,
configurable retention/schedule — see `docs/DEPLOYMENT.md` §6):

```bash
sudo ../../scripts/install-backup-cron.sh --schedule "15 3 * * *" --retention-days 14
```

## Tests

```bash
go test ./...                              # unit tests (no database required)

# Optional: database-backed integration tests
export WR_TEST_DATABASE_URL="postgres://user:pass@127.0.0.1:5432/wartungsremote_test?sslmode=disable"
go test ./tests/... -v
```

## Security

Read `docs/SECURITY.md` before changing anything security-relevant. Report
vulnerabilities per the top-level [`SECURITY.md`](SECURITY.md).

**Deliberate deviation from the V1 least-privilege default:** the Linux
agent (`scripts/install-agent-linux.sh`,
`deployment/systemd/wartungsremote-agent.service`) runs as **root**, not
the unprivileged dedicated service account `docs/SECURITY.md` §18
otherwise describes. This is an intentional tradeoff, not an oversight —
it lets remote support fully administer a device without ever needing
the customer's own credentials. It also means a compromised wr-core
server, or a bug in the agent's own command handling, is immediately
root on every enrolled Linux device. The required mitigation: the admin
dashboard (`admin.listen`) must never be reachable from the public
internet — only the agent-facing gateway (`public.listen`) is, and it
never exposes a login. See `docs/DEPLOYMENT.md` §2-3. The Windows agent
reaches the same capability level via the Windows Service running as
LocalSystem — no separate decision needed there.

## License

Source-available, not open source — see [`LICENSE`](LICENSE). You may run
your own instance for your own devices; hosting it as a service for third
parties needs separate permission.

---

Powered by [sonnyathome.online](https://sonnyathome.online)
