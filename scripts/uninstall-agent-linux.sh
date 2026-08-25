#!/usr/bin/env bash
# Uninstalls wr-agent per docs/AGENT.md §17. The device stays registered on
# the server (status will move to offline); revoke it separately in the
# dashboard if it should be fully retired.
set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "This script must be run as root (sudo)." >&2
  exit 1
fi

systemctl stop wartungsremote-agent 2>/dev/null || true
systemctl disable wartungsremote-agent 2>/dev/null || true
rm -f /etc/systemd/system/wartungsremote-agent.service
systemctl daemon-reload

rm -f /usr/local/bin/wr-agent

read -r -p "Remove configuration and stored device credential too? [y/N] " confirm
if [[ "${confirm,,}" == "y" ]]; then
  rm -rf /etc/wartungsremote /var/lib/wartungsremote /var/log/wartungsremote
fi

echo "wr-agent uninstalled."
