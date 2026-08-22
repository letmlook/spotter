#!/usr/bin/env bash
# scripts/deploy.sh — macOS / Linux
#
# Deploys spotterd to a target Linux device over SSH.
# Uploads the matching-arch binary, systemd unit, and install.sh,
# then runs install.sh on the target.
#
# Usage:
#   scripts/deploy.sh <user> <password> <ip> [arm64|amd64]
#
# Requires:
#   - sshpass
#       macOS:    brew install hudochenkov/sshpass/sshpass
#       Debian:   sudo apt install sshpass
#       Fedora:   sudo dnf install sshpass
#   - The SSH user must be passwordless-sudo on the target (so the
#     install script can run `sudo bash /tmp/install.sh` without an
#     interactive password prompt). If that isn't the case, run the
#     manual scp/ssh steps from the README instead.
#
# Environment overrides:
#   BIN_DIR    directory holding spotterd-linux-* binaries
#              (default: <repo>/bin)
#   SCRIPT_SRC directory holding spotterd.service + install.sh
#              (default: <repo>/scripts)
#   SSH_PORT   target SSH port (default: 22)

set -uo pipefail

if [ "$#" -lt 3 ] || [ "$#" -gt 4 ]; then
  echo "usage: $0 <user> <password> <ip> [arm64|amd64]" >&2
  exit 2
fi

USER="$1"
PASS="$2"
IP="$3"
ARCH="${4:-arm64}"
PORT="${SSH_PORT:-22}"

# Resolve paths relative to repo root (script lives in scripts/).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="${BIN_DIR:-$ROOT/bin}"
SCRIPT_SRC="${SCRIPT_SRC:-$ROOT/scripts}"

case "$ARCH" in
  arm64) BIN="$BIN_DIR/spotterd-linux-arm64" ;;
  amd64) BIN="$BIN_DIR/spotterd-linux-x64" ;;
  *)
    echo "error: arch must be arm64 or amd64 (got: $ARCH)" >&2
    exit 2
    ;;
esac

if [ ! -f "$BIN" ]; then
  suffix="$([ "$ARCH" = "amd64" ] && echo x64 || echo arm64)"
  echo "error: missing binary $BIN (run 'make agent-linux-$suffix' first)" >&2
  exit 1
fi

if ! command -v sshpass >/dev/null 2>&1; then
  cat >&2 <<EOF
error: sshpass not found.

  install on macOS:                  brew install hudochenkov/sshpass/sshpass
  install on Debian / Ubuntu:        sudo apt install sshpass
  install on Fedora / RHEL / Rocky:  sudo dnf install sshpass
  install on Arch:                   sudo pacman -S sshpass

After installing, re-run this script.
EOF
  exit 1
fi

export SSHPASS="$PASS"
SSH_TARGET="$USER@$IP"
SSH_OPTS=(-P "$PORT" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR)

log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[ ok]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[fail]\033[0m %s\n' "$*" >&2; exit 1; }

log "uploading spotterd ($ARCH) -> $IP:$PORT"
sshpass -e scp "${SSH_OPTS[@]}" "$BIN" "$SSH_TARGET:/tmp/spotterd" \
  || fail "scp spotterd"

log "uploading systemd unit"
sshpass -e scp "${SSH_OPTS[@]}" "$SCRIPT_SRC/spotterd.service" \
  "$SSH_TARGET:/tmp/spotterd.service" \
  || fail "scp unit"

log "uploading install.sh"
sshpass -e scp "${SSH_OPTS[@]}" "$SCRIPT_SRC/install.sh" \
  "$SSH_TARGET:/tmp/install.sh" \
  || fail "scp install.sh"

log "running install.sh on $IP (sudo)"
sshpass -e ssh "${SSH_OPTS[@]}" "$SSH_TARGET" "sudo bash /tmp/install.sh" \
  || fail "install.sh"
ok "spotterd installed on $IP"

log "verifying with /healthz"
if sshpass -e ssh "${SSH_OPTS[@]}" "$SSH_TARGET" \
     "curl -fsS http://127.0.0.1:9999/healthz" >/dev/null 2>&1; then
  ok "$IP responds on :9999/healthz"
else
  warn "$IP did not respond on :9999/healthz — install succeeded but service may need a moment to start."
fi