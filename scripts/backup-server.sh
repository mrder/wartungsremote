#!/usr/bin/env bash
# Backs up everything docs/DEPLOYMENT.md §6 requires for a Docker Compose
# deployment: the Postgres database, server config, and the secret files
# without which a DB backup is only partially usable (session pepper,
# TOTP encryption key — without them, existing sessions/TOTP enrollments
# in a restored DB simply won't validate; docs/DEPLOYMENT.md §6).
#
# Meant to run unattended from cron (see install-backup-cron.sh, which
# sets this up for you), but safe to run by hand too. Every setting below
# is a variable on purpose — nothing here is hardcoded to one server's
# layout, so the same script works whether the Docker Compose stack lives
# in the default deployment/docker/ or somewhere else entirely.
set -euo pipefail

# --- Configurable settings (env var, or --flag, or the default below) ------
COMPOSE_DIR="${WR_BACKUP_COMPOSE_DIR:-$(cd "$(dirname "$0")/../deployment/docker" && pwd)}"
BACKUP_DIR="${WR_BACKUP_DIR:-/var/backups/wartungsremote}"
RETENTION_DAYS="${WR_BACKUP_RETENTION_DAYS:-14}"
ENCRYPT_PASSPHRASE_FILE="${WR_BACKUP_ENCRYPT_PASSPHRASE_FILE:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --compose-dir) COMPOSE_DIR="$2"; shift 2 ;;
    --backup-dir) BACKUP_DIR="$2"; shift 2 ;;
    --retention-days) RETENTION_DAYS="$2"; shift 2 ;;
    --encrypt-passphrase-file) ENCRYPT_PASSPHRASE_FILE="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

if [[ ! -f "$COMPOSE_DIR/docker-compose.yml" ]]; then
  echo "backup-server: no docker-compose.yml found in $COMPOSE_DIR (--compose-dir / WR_BACKUP_COMPOSE_DIR)" >&2
  exit 1
fi

TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
mkdir -p "$BACKUP_DIR"
umask 077

echo "Dumping database..."
docker compose --project-directory "$COMPOSE_DIR" exec -T postgres \
  pg_dump -U wartungsremote wartungsremote | gzip > "$WORKDIR/database.sql.gz"

echo "Collecting config and secrets..."
mkdir -p "$WORKDIR/config"
for f in server.yaml .env; do
  [[ -f "$COMPOSE_DIR/$f" ]] && cp "$COMPOSE_DIR/$f" "$WORKDIR/config/"
done
[[ -d "$COMPOSE_DIR/secrets" ]] && cp -r "$COMPOSE_DIR/secrets" "$WORKDIR/config/secrets"

# A human-readable record of when/what/where this backup is from, inside
# the archive itself — the filename's timestamp is lost the moment someone
# renames or copies the file elsewhere, this isn't.
cat > "$WORKDIR/MANIFEST.txt" <<MANIFEST
WartungsRemote backup
created_at:   $(date -u +"%Y-%m-%d %H:%M:%S UTC")
timestamp_id: $TIMESTAMP
deployment:   docker compose
hostname:     $(hostname)
compose_dir:  $COMPOSE_DIR
MANIFEST

ARCHIVE="$BACKUP_DIR/wartungsremote-backup-$TIMESTAMP.tar.gz"
tar -czf "$ARCHIVE" -C "$WORKDIR" database.sql.gz config MANIFEST.txt

if [[ -n "$ENCRYPT_PASSPHRASE_FILE" ]]; then
  if [[ ! -f "$ENCRYPT_PASSPHRASE_FILE" ]]; then
    echo "backup-server: --encrypt-passphrase-file $ENCRYPT_PASSPHRASE_FILE not found" >&2
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
