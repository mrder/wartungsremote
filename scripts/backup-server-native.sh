#!/usr/bin/env bash
# Backs up everything docs/DEPLOYMENT.md §6 requires, for a *native*
# (non-Docker) install: the Postgres database, server.yaml, and the
# secret files without which a DB backup is only partially usable
# (session pepper, TOTP encryption key — without them, existing
# sessions/TOTP enrollments in a restored DB simply won't validate).
#
# Counterpart to backup-server.sh, which assumes a Docker Compose
# deployment and shells out to `docker compose exec postgres pg_dump`;
# this one runs pg_dump directly against the DSN in the app's own
# secrets/database_url.txt (the migration DSN — it has full read access,
# unlike the restricted runtime role, which is exactly what a full dump
# needs). Same encrypt/retention flags, same reasoning, different source
# database and different files to collect.
set -euo pipefail

# --- Configurable settings (env var, or --flag, or the default below) ------
APP_DIR="${WR_BACKUP_APP_DIR:-$(cd "$(dirname "$0")/.." && pwd)}"
BACKUP_DIR="${WR_BACKUP_DIR:-/var/backups/wartungsremote}"
RETENTION_DAYS="${WR_BACKUP_RETENTION_DAYS:-14}"
ENCRYPT_PASSPHRASE_FILE="${WR_BACKUP_ENCRYPT_PASSPHRASE_FILE:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --app-dir) APP_DIR="$2"; shift 2 ;;
    --backup-dir) BACKUP_DIR="$2"; shift 2 ;;
    --retention-days) RETENTION_DAYS="$2"; shift 2 ;;
    --encrypt-passphrase-file) ENCRYPT_PASSPHRASE_FILE="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

DSN_FILE="$APP_DIR/secrets/database_url.txt"
if [[ ! -f "$DSN_FILE" ]]; then
  echo "backup-server-native: no $DSN_FILE found (--app-dir / WR_BACKUP_APP_DIR)" >&2
  exit 1
fi

TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
mkdir -p "$BACKUP_DIR"
umask 077

echo "Dumping database..."
pg_dump "$(cat "$DSN_FILE")" | gzip > "$WORKDIR/database.sql.gz"

echo "Collecting config and secrets..."
mkdir -p "$WORKDIR/config"
[[ -f "$APP_DIR/server.yaml" ]] && cp "$APP_DIR/server.yaml" "$WORKDIR/config/"
[[ -d "$APP_DIR/secrets" ]] && cp -r "$APP_DIR/secrets" "$WORKDIR/config/secrets"

ARCHIVE="$BACKUP_DIR/wartungsremote-backup-$TIMESTAMP.tar.gz"
tar -czf "$ARCHIVE" -C "$WORKDIR" database.sql.gz config

if [[ -n "$ENCRYPT_PASSPHRASE_FILE" ]]; then
  if [[ ! -f "$ENCRYPT_PASSPHRASE_FILE" ]]; then
    echo "backup-server-native: --encrypt-passphrase-file $ENCRYPT_PASSPHRASE_FILE not found" >&2
    exit 1
  fi
  openssl enc -aes-256-cbc -pbkdf2 -salt -in "$ARCHIVE" -out "$ARCHIVE.enc" -pass "file:$ENCRYPT_PASSPHRASE_FILE"
  rm -f "$ARCHIVE"
  ARCHIVE="$ARCHIVE.enc"
  echo "Encrypted with AES-256-CBC (passphrase file: $ENCRYPT_PASSPHRASE_FILE)"
fi

chmod 600 "$ARCHIVE"
echo "Backup written: $ARCHIVE ($(du -h "$ARCHIVE" | cut -f1))"

if [[ "$RETENTION_DAYS" -gt 0 ]]; then
  DELETED=$(find "$BACKUP_DIR" -maxdepth 1 -name 'wartungsremote-backup-*.tar.gz*' -mtime "+$RETENTION_DAYS" -print -delete | wc -l)
  # A plain `[[ ... ]] && echo ...` here would leave the script exiting 1
  # (from the false `[[ ]]`) on the common case of nothing to prune — cron
  # would then report every routine backup as a failure. The `if` form's
  # own exit status is always 0 regardless of which branch ran.
  if [[ "$DELETED" -gt 0 ]]; then
    echo "Pruned $DELETED backup(s) older than $RETENTION_DAYS day(s)"
  fi
fi
