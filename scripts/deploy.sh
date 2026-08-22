#!/usr/bin/env bash
# scripts/deploy.sh — macOS / Linux
#
# Deploys spotterd to a target Linux device over SSH.
# Uploads the matching-arch binary, systemd unit, and install.sh,
# then runs install.sh on the target.
#
# Usage:
#   # SSH-key auth (no password):
#   scripts/deploy.sh <user> <ip> [arm64|amd64]
#
#   # Password auth:
#   scripts/deploy.sh <user> <password> <ip> [arm64|amd64]
#
#   # Password can also be the literal empty string "" to force
#   # key auth while keeping the 4-arg form for scripting:
#   scripts/deploy.sh <user> "" <ip> [arm64|amd64]
#
# The form is auto-detected: if the 2nd argument looks like an IPv4
# address (dotted-quad), it's treated as <ip> and key auth is used.
# Otherwise the script expects the 4-arg form with an explicit
# password.
#
# Requires for password mode:
#   - sshpass
#       macOS:    brew install hudochenkov/sshpass/sshpass
#       Debian:   sudo apt install sshpass
#       Fedora:   sudo dnf install sshpass
#   - The SSH user must be passwordless-sudo on the target (so the
#     install script can run `sudo bash /tmp/install.sh` without an
#     interactive password prompt). If that isn't the case, run the
#     manual scp/ssh steps from the README instead.
#
# Requires for key-auth mode:
#   - A working SSH key loaded in ssh-agent (or the default
#     ~/.ssh/id_rsa / id_ed25519) that the target accepts.
#   - Same passwordless-sudo constraint applies once the script
#     gets past ssh.
#
# Environment overrides:
#   BIN_DIR    directory holding spotterd-linux-* binaries
#              (default: <repo>/bin)
#   SCRIPT_SRC directory holding spotterd.service + install.sh
#              (default: <repo>/scripts)
#   SSH_PORT   target SSH port (default: 22)

set -uo pipefail

if [ "$#" -lt 2 ] || [ "$#" -gt 4 ]; then
  cat >&2 <<EOF
usage:
  $0 <user> <ip> [arm64|amd64]                 # SSH key auth
  $0 <user> <password> <ip> [arm64|amd64]     # password auth
EOF
  exit 2
fi

# Detect 3-arg form (key auth) vs 4-arg form (password auth) by
# looking at whether arg 2 is a dotted-quad IPv4 literal.
if [[ "$2" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]]; then
  USER="$1"
  IP="$2"
  ARCH="${3:-arm64}"
  PASS=""
else
  if [ "$#" -lt 3 ]; then
    echo "usage: $0 <user> <password> <ip> [arm64|amd64]" >&2
    exit 2
  fi
  USER="$1"
  PASS="$2"
  IP="$3"
  ARCH="${4:-arm64}"
fi

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

if [ -n "$PASS" ] && ! command -v sshpass >/dev/null 2>&1; then
  cat >&2 <<EOF
error: sshpass not found.

  install on macOS:                  brew install hudochenkov/sshpass/sshpass
  install on Debian / Ubuntu:        sudo apt install sshpass
  install on Fedora / RHEL / Rocky:  sudo dnf install sshpass
  install on Arch:                   sudo pacman -S sshpass

Tip: drop the password and let ssh use your keychain instead.
  $0 $USER $IP $ARCH
EOF
  exit 1
fi

SSH_TARGET="$USER@$IP"
SSH_OPTS=(-P "$PORT" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR)

# Wrappers that switch between sshpass-wrapped and plain scp/ssh
# depending on whether a password was supplied.
run_scp() {
  if [ -n "$PASS" ]; then
    SSHPASS="$PASS" sshpass -e scp "$@"
  else
    scp "$@"
  fi
}
run_ssh() {
  if [ -n "$PASS" ]; then
    SSHPASS="$PASS" sshpass -e ssh "$@"
  else
    ssh "$@"
  fi
}

log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[ ok]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[warn]\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31m[fail]\033[0m %s\n' "$*" >&2; exit 1; }

if [ -z "$PASS" ]; then
  log "no password supplied — using SSH key auth"
else
  log "password supplied — using sshpass"
fi

log "uploading spotterd ($ARCH) -> $IP:$PORT"
run_scp "${SSH_OPTS[@]}" "$BIN" "$SSH_TARGET:/tmp/spotterd" \
  || fail "scp spotterd"

log "uploading systemd unit"
run_scp "${SSH_OPTS[@]}" "$SCRIPT_SRC/spotterd.service" \
  "$SSH_TARGET:/tmp/spotterd.service" \
  || fail "scp unit"

log "uploading install.sh"
run_scp "${SSH_OPTS[@]}" "$SCRIPT_SRC/install.sh" \
  "$SSH_TARGET:/tmp/install.sh" \
  || fail "scp install.sh"

log "running install.sh on $IP (sudo)"
run_ssh "${SSH_OPTS[@]}" "$SSH_TARGET" "sudo bash /tmp/install.sh" \
  || fail "install.sh"
ok "spotterd installed on $IP"

log "verifying with /healthz"
if run_ssh "${SSH_OPTS[@]}" "$SSH_TARGET" \
     "curl -fsS http://127.0.0.1:9999/healthz" >/dev/null 2>&1; then
  ok "$IP responds on :9999/healthz"
else
  warn "$IP did not respond on :9999/healthz — install succeeded but service may need a moment to start."
fi