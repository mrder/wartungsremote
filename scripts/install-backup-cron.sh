#!/usr/bin/env bash
# Installs (or updates) a cron job that runs scripts/backup-server.sh on a
# schedule. Everything is a flag/env var — nothing here forces one fixed
# schedule or backup location, per docs/DEPLOYMENT.md §6 ("mindestens
# täglich"), that's just the default.
#
# Usage:
#   sudo ./scripts/install-backup-cron.sh \
#     --schedule "15 3 * * *" \
#     --backup-dir /var/backups/wartungsremote \
#     --retention-days 14 \
#     [--encrypt-passphrase-file /path/to/passphrase.txt]
#
# Re-run any time to change the schedule or settings — it replaces its own
# previous crontab line instead of adding a duplicate.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCHEDULE="15 3 * * *"
BACKUP_DIR="/var/backups/wartungsremote"
RETENTION_DAYS="14"
COMPOSE_DIR=""
ENCRYPT_PASSPHRASE_FILE=""
LOG_FILE="/var/log/wartungsremote-backup.log"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --schedule) SCHEDULE="$2"; shift 2 ;;
    --backup-dir) BACKUP_DIR="$2"; shift 2 ;;
    --retention-days) RETENTION_DAYS="$2"; shift 2 ;;
    --compose-dir) COMPOSE_DIR="$2"; shift 2 ;;
    --encrypt-passphrase-file) ENCRYPT_PASSPHRASE_FILE="$2"; shift 2 ;;
    --log-file) LOG_FILE="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

if [[ $EUID -ne 0 ]]; then
  echo "This installer must be run as root (sudo) — it writes to root's crontab." >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"
umask 077

ENV_FILE="$BACKUP_DIR/backup.env"
{
  echo "WR_BACKUP_DIR=$BACKUP_DIR"
  echo "WR_BACKUP_RETENTION_DAYS=$RETENTION_DAYS"
  [[ -n "$COMPOSE_DIR" ]] && echo "WR_BACKUP_COMPOSE_DIR=$COMPOSE_DIR"
  [[ -n "$ENCRYPT_PASSPHRASE_FILE" ]] && echo "WR_BACKUP_ENCRYPT_PASSPHRASE_FILE=$ENCRYPT_PASSPHRASE_FILE"
} > "$ENV_FILE"
echo "Wrote settings to $ENV_FILE (edit this file directly to change them without re-running this installer)"

CRON_MARKER="# wartungsremote-backup (managed by install-backup-cron.sh)"
CRON_LINE="$SCHEDULE . $ENV_FILE; $SCRIPT_DIR/backup-server.sh >> $LOG_FILE 2>&1 $CRON_MARKER"

( crontab -l 2>/dev/null | grep -vF "$CRON_MARKER" ; echo "$CRON_LINE" ) | crontab -

echo "Installed cron schedule: $SCHEDULE"
echo "Logs: $LOG_FILE"
echo "Run it once now to verify: sudo bash -c '. $ENV_FILE; $SCRIPT_DIR/backup-server.sh'"
