#!/usr/bin/env bash
# Installs wr-core as a systemd service for native (non-Docker) deployment.
# Requires a reachable PostgreSQL instance; see docs/DEPLOYMENT.md. For most
# production setups, deployment/docker/docker-compose.yml is preferred.
#
# Usage:
#   sudo ./install-core-linux.sh --database-url "postgres://wruser:pass@localhost:5432/wartungsremote"
set -euo pipefail

DATABASE_URL=""
BINARY_SRC="$(dirname "$0")/../wr-core"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --database-url) DATABASE_URL="$2"; shift 2 ;;
    --binary) BINARY_SRC="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

if [[ $EUID -ne 0 ]]; then
  echo "This installer must be run as root (sudo)." >&2
  exit 1
fi
if [[ -z "$DATABASE_URL" ]]; then
  echo "--database-url is required" >&2
  exit 1
fi
if [[ ! -f "$BINARY_SRC" ]]; then
  echo "wr-core binary not found at $BINARY_SRC (build it first: go build ./cmd/wr-core)" >&2
  exit 1
fi

id -u wartungsremote-core >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin wartungsremote-core

install -d -o wartungsremote-core -g wartungsremote-core -m 0750 /etc/wartungsremote
install -d -o wartungsremote-core -g wartungsremote-core -m 0700 /etc/wartungsremote/secrets
install -d -o wartungsremote-core -g wartungsremote-core -m 0750 /var/log/wartungsremote-core
# Not every distro ships /usr/local/bin pre-created (e.g. ZimaOS) — install
# doesn't create missing parent directories on its own.
install -d -o root -g root -m 0755 /usr/local/bin

install -o root -g root -m 0755 "$BINARY_SRC" /usr/local/bin/wr-core

if [[ ! -f /etc/wartungsremote/server.yaml ]]; then
  install -o wartungsremote-core -g wartungsremote-core -m 0640 \
    "$(dirname "$0")/../deployment/docker/server.example.yaml" /etc/wartungsremote/server.yaml
fi

umask 077
echo -n "$DATABASE_URL" > /etc/wartungsremote/secrets/database_url
[[ -f /etc/wartungsremote/secrets/session_pepper ]] || head -c 32 /dev/urandom > /etc/wartungsremote/secrets/session_pepper
[[ -f /etc/wartungsremote/secrets/totp_key ]] || head -c 32 /dev/urandom > /etc/wartungsremote/secrets/totp_key
chown -R wartungsremote-core:wartungsremote-core /etc/wartungsremote/secrets
chmod 600 /etc/wartungsremote/secrets/*

cat > /etc/wartungsremote/core.env <<EOF
WR_DATABASE_URL_FILE=/etc/wartungsremote/secrets/database_url
WR_SESSION_PEPPER_FILE=/etc/wartungsremote/secrets/session_pepper
WR_TOTP_ENCRYPTION_KEY_FILE=/etc/wartungsremote/secrets/totp_key
EOF
chown wartungsremote-core:wartungsremote-core /etc/wartungsremote/core.env
chmod 600 /etc/wartungsremote/core.env

install -o root -g root -m 0644 "$(dirname "$0")/../deployment/systemd/wartungsremote-core.service" /etc/systemd/system/wartungsremote-core.service

systemctl daemon-reload
systemctl enable wartungsremote-core
systemctl restart wartungsremote-core

echo "wr-core installed. Check status with: systemctl status wartungsremote-core"
echo "Create the first super_admin with:"
echo "  sudo -u wartungsremote-core WR_DATABASE_URL_FILE=/etc/wartungsremote/secrets/database_url \\"
echo "    /usr/local/bin/wr-core createadmin --username admin --password-file <path> --config /etc/wartungsremote/server.yaml"
