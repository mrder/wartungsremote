#!/usr/bin/env bash
# WartungsRemote agent one-line installer for Linux. Looks up the latest
# signed agent-* GitHub release, downloads the matching binary for this
# machine's architecture, and installs it as a systemd service via
# scripts/install-agent-linux.sh (unmodified — this script only fetches
# what that one needs, it doesn't reimplement it).
#
# Generated for you with server URL + token already filled in by the
# dashboard's "+ Add Device" panel. Safe to copy/paste as one line:
#
#   curl -fsSL https://raw.githubusercontent.com/mrder/wartungsremote/main/scripts/quickinstall-agent-linux.sh \
#     | sudo bash -s -- --server-url https://remote.example.de --token wr_enroll_XXXXXXXX
#
# --channel stable (default) installs the newest agent-* release that is
# NOT marked as a GitHub pre-release. --channel beta installs the newest
# agent-* release regardless of pre-release status (may be the same as
# stable if no beta is currently published).
set -euo pipefail

REPO="mrder/wartungsremote"
SERVER_URL=""
TOKEN=""
CHANNEL="stable"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-url) SERVER_URL="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --repo) REPO="$2"; shift 2 ;;
    --channel) CHANNEL="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

if [[ "$CHANNEL" != "stable" && "$CHANNEL" != "beta" ]]; then
  echo "--channel must be 'stable' or 'beta', got: $CHANNEL" >&2
  exit 1
fi

if [[ $EUID -ne 0 ]]; then
  echo "This installer must be run as root (sudo)." >&2
  exit 1
fi
if [[ -z "$SERVER_URL" || -z "$TOKEN" ]]; then
  echo "usage: quickinstall-agent-linux.sh --server-url <url> --token <wr_enroll_...>" >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

echo "Looking up the latest signed agent release (channel: ${CHANNEL})..."
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases" | awk -v channel="$CHANNEL" '
  /"tag_name":/ {
    line = $0
    gsub(/.*"tag_name": *"/, "", line)
    gsub(/".*/, "", line)
    tag = line
  }
  /"prerelease":/ {
    pre = ($0 ~ /true/)
    if (tag ~ /^agent-/ && !found) {
      if (channel == "beta" || !pre) {
        print tag
        found = 1
      }
    }
    tag = ""
  }
')
if [[ -z "$TAG" ]]; then
  echo "could not find a published agent-* release (channel: ${CHANNEL}) under ${REPO}" >&2
  exit 1
fi
echo "Installing ${TAG} for linux/${GOARCH}"

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT
cd "$WORKDIR"

mkdir -p scripts deployment/systemd
curl -fsSL -o wr-agent "https://github.com/${REPO}/releases/download/${TAG}/wr-agent-linux-${GOARCH}"
curl -fsSL -o scripts/install-agent-linux.sh "https://raw.githubusercontent.com/${REPO}/main/scripts/install-agent-linux.sh"
curl -fsSL -o deployment/systemd/wartungsremote-agent.service "https://raw.githubusercontent.com/${REPO}/main/deployment/systemd/wartungsremote-agent.service"
chmod +x wr-agent scripts/install-agent-linux.sh

bash scripts/install-agent-linux.sh --server-url "$SERVER_URL" --token "$TOKEN" --binary "$WORKDIR/wr-agent"

echo "Done. Check status with: systemctl status wartungsremote-agent"
