#!/usr/bin/env bash
# Installs wr-agent as a systemd service on Debian/Ubuntu/Raspberry Pi OS.
# See docs/AGENT.md and docs/TODO.md Phase 26.
#
# Runs as root (deliberate choice, not the V1-documented unprivileged
# default): remote support must be able to fully administer the device
# without ever needing the customer's own login. This trades a larger
# blast radius (a compromised wr-core server or a bug in the agent's own
# command handling is immediately root on every enrolled device) for
# operational simplicity. Mitigate at the network layer — the admin
# dashboard must stay off the public internet (loopback/VPN-only, see
# docs/DEPLOYMENT.md §2-3); only the agent-facing gateway is ever
# internet-reachable.
#
# Usage:
#   sudo ./install-agent-linux.sh --server-url https://remote.example.de --token wr_enroll_XXXXXXXX
#
set -euo pipefail

SERVER_URL=""
TOKEN=""
BINARY_SRC="$(dirname "$0")/../wr-agent"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-url) SERVER_URL="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --binary) BINARY_SRC="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

if [[ $EUID -ne 0 ]]; then
  echo "This installer must be run as root (sudo)." >&2
  exit 1
fi
if [[ -z "$SERVER_URL" ]]; then
  echo "--server-url is required" >&2
  exit 1
fi
if [[ ! -f "$BINARY_SRC" ]]; then
  echo "wr-agent binary not found at $BINARY_SRC (build it first: GOOS=linux go build ./cmd/wr-agent)" >&2
  exit 1
fi

install -d -o root -g root -m 0700 /etc/wartungsremote
install -d -o root -g root -m 0700 /var/lib/wartungsremote
install -d -o root -g root -m 0750 /var/log/wartungsremote

install -o root -g root -m 0755 "$BINARY_SRC" /usr/local/bin/wr-agent

if [[ ! -f /etc/wartungsremote/agent.yaml ]]; then
  cat > /etc/wartungsremote/agent.yaml <<EOF
server_url: ${SERVER_URL}
update_channel: stable
log_level: info
policy:
  terminal: true
  ssh_tunnel: true
  rdp_tunnel: true
  files_read: true
  files_write: true
  service_control: true
  process_terminate: true
  power_control: true
EOF
  chown root:root /etc/wartungsremote/agent.yaml
  chmod 0600 /etc/wartungsremote/agent.yaml
fi

if [[ -n "$TOKEN" ]]; then
  echo -n "$TOKEN" > /var/lib/wartungsremote/enroll.token
  chown root:root /var/lib/wartungsremote/enroll.token
  chmod 0600 /var/lib/wartungsremote/enroll.token
fi

install -o root -g root -m 0644 "$(dirname "$0")/../deployment/systemd/wartungsremote-agent.service" /etc/systemd/system/wartungsremote-agent.service

systemctl daemon-reload
systemctl enable wartungsremote-agent
systemctl restart wartungsremote-agent

echo "wr-agent installed. Check status with: systemctl status wartungsremote-agent"
