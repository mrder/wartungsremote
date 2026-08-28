#!/usr/bin/env bash
# Uninstalls the native (non-Docker) wr-core systemd service. The PostgreSQL
# database itself is never touched by this script.
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "This script must be run as root (sudo)." >&2
  exit 1
fi

systemctl stop wartungsremote-core 2>/dev/null || true
systemctl disable wartungsremote-core 2>/dev/null || true
rm -f /etc/systemd/system/wartungsremote-core.service
systemctl daemon-reload

rm -f /var/lib/wartungsremote-core/wr-core

read -r -p "Remove configuration and secrets too (NOT the database)? [y/N] " confirm
if [[ "${confirm,,}" == "y" ]]; then
  rm -rf /etc/wartungsremote /var/log/wartungsremote-core /var/lib/wartungsremote-core
fi

echo "wr-core uninstalled."
