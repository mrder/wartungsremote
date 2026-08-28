#!/usr/bin/env bash
# Generates the local secret files referenced by deployment/docker/docker-compose.yml.
# Run once before the first `docker compose up`. These files are gitignored.
set -euo pipefail

DIR="$(dirname "$0")/../deployment/docker/secrets"
mkdir -p "$DIR"
umask 077

if [[ ! -f "$DIR/db_password.txt" ]]; then
  openssl rand -base64 24 | tr -d '\n' > "$DIR/db_password.txt"
fi

if [[ ! -f "$DIR/database_url.txt" ]]; then
  DB_PASS="$(cat "$DIR/db_password.txt")"
  printf 'postgres://wartungsremote:%s@postgres:5432/wartungsremote?sslmode=disable' "$DB_PASS" > "$DIR/database_url.txt"
fi

# Least-privilege runtime role (docs/DEPLOYMENT.md §5a) — db-init creates/
# updates it on every stack startup from this same password.
if [[ ! -f "$DIR/app_role_password.txt" ]]; then
  openssl rand -base64 24 | tr -d '\n' > "$DIR/app_role_password.txt"
fi
if [[ ! -f "$DIR/runtime_database_url.txt" ]]; then
  APP_PASS="$(cat "$DIR/app_role_password.txt")"
  printf 'postgres://wartungsremote_app:%s@postgres:5432/wartungsremote?sslmode=disable' "$APP_PASS" > "$DIR/runtime_database_url.txt"
fi

[[ -f "$DIR/session_pepper.bin" ]] || head -c 32 /dev/urandom > "$DIR/session_pepper.bin"
[[ -f "$DIR/totp_key.bin" ]] || head -c 32 /dev/urandom > "$DIR/totp_key.bin"

if [[ ! -f "$DIR/admin_password.txt" ]]; then
  openssl rand -base64 18 | tr -d '\n' > "$DIR/admin_password.txt"
fi

echo "Secrets generated in $DIR"
echo "First admin account password: $DIR/admin_password.txt (used by the createadmin step in the README — change it after first login)"
