#!/usr/bin/env bash
# scripts/build-all.sh
#
# One-click build and package script. Designed to run on ANY of the
# three supported client platforms (Windows, macOS, Linux); the script
# detects the host OS/arch and produces the matching client binary
# alongside the agent binaries.
#
# What it does:
#   1. Builds spotterd for both Linux arm64 and amd64 (Go cross-compile
#      works from any host).
#   2. Builds spotter-client for the CURRENT host platform only. On
#      macOS the .app bundle is produced; on Linux a plain binary; on
#      Windows a .exe. To ship binaries for every platform, run this
#      script once on each platform.
#   3. Copies the systemd unit plus the manual install/uninstall/
#      cleanup shell scripts alongside the binaries, and writes a
#      SHA256SUMS file. These scripts are stand-alone utilities — the
#      GUI client no longer drives them — so users who SSH into a
#      device by hand have everything they need in one tarball.
#
# Output layout:
#   dist/
#   ├── agents/
#   │   ├── spotterd-linux-arm64
#   │   └── spotterd-linux-x64
#   ├── clients/
#   │   └── ...              # exactly one entry — the host's client
#   ├── scripts/
#   │   ├── install.sh       # manual device-side installer
#   │   ├── uninstall.sh     # manual device-side uninstaller
#   │   ├── cleanup.sh       # best-effort cleanup after a failed install
#   │   ├── deploy.sh        # macOS / Linux deploy helper (sshpass)
#   │   ├── deploy.ps1       # Windows deploy helper (plink + pscp)
#   │   └── spotterd.service # systemd unit (always needed)
#   ├── README.md
#   └── SHA256SUMS
#
# Usage:
#   scripts/build-all.sh              # full build (agents + host client)
#   scripts/build-all.sh --agent-only # agents only, skip client
#
# Environment:
#   OUT_DIR  override the output directory (default: <repo>/dist)
#   GO       override the Go binary (default: go)

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT_DIR="${OUT_DIR:-$PROJECT_ROOT/dist}"
AGENT_OUT="$OUT_DIR/agents"
CLIENT_OUT="$OUT_DIR/clients"
SCRIPT_OUT="$OUT_DIR/scripts"

HOST_OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
HOST_ARCH="$(uname -m)"
# Normalise arch names to the wails convention (amd64 / arm64).
case "$HOST_ARCH" in
  x86_64)  HOST_ARCH=amd64 ;;
  aarch64) HOST_ARCH=arm64 ;;
  arm64)   HOST_ARCH=arm64 ;;
esac

GO="${GO:-go}"
WAILS="$(command -v wails 2>/dev/null || true)"

# ---------- argument parsing ----------
AGENT_ONLY=false
for arg in "$@"; do
  case "$arg" in
    --agent-only) AGENT_ONLY=true ;;
    -h|--help)
      sed -n '2,/^set -/p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *)
      echo "error: unknown argument: $arg" >&2
      exit 2
      ;;
  esac
done

# ---------- helpers ----------
log()  { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[ ok]\033[0m %s\n' "$*"; }

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# ---------- agent builds ----------
# `arch` is the Go arch (arm64 / amd64); `fname` is the on-disk
# artefact name (arm64 / x64) to match the Makefile and README
# convention.
build_agent() {
  local arch="$1"
  local fname="$([ "$arch" = "amd64" ] && echo x64 || echo arm64)"
  local src="$PROJECT_ROOT/bin/spotterd-linux-$fname"
  log "spotterd linux/$arch"
  # Env-var assignments must precede the command for bash to honour
  # them; placing them after `go build` makes them be parsed as package
  # paths (which is what produced the earlier "malformed import path"
  # errors).
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
    "$GO" build -trimpath -o "$src" ./cmd/agent
  install -m 0755 "$src" "$AGENT_OUT/spotterd-linux-$fname"
  ok "spotterd linux/$arch"
}

# ---------- client build (host only) ----------
# Builds the client for the current host platform. Produces either a
# .app bundle (macOS), an .exe (Windows), or a plain binary (Linux),
# staged under dist/clients/ with the canonical project naming.
build_client_host() {
  log "spotter-client $HOST_OS/$HOST_ARCH"
  if [ -n "$WAILS" ]; then
    # Native wails build honours host GOOS.
    "$WAILS" build -o "$PROJECT_ROOT/build/bin" >/dev/null
  else
    # Fallback: build frontend then `go build` for the host.
    if [ ! -d "$PROJECT_ROOT/frontend/node_modules" ]; then
      (cd "$PROJECT_ROOT/frontend" && npm install) >/dev/null
    fi
    (cd "$PROJECT_ROOT/frontend" && npm run build >/dev/null 2>&1)
    local out="$CLIENT_OUT/spotter-client-$HOST_OS-$HOST_ARCH"
    [ "$HOST_OS" = "windows" ] && out="$out.exe"
    "$GO" build -trimpath -o "$out" .
    return 0
  fi
  stage_client_artifacts
  ok "spotter-client $HOST_OS/$HOST_ARCH"
}

stage_client_artifacts() {
  local build_bin="$PROJECT_ROOT/build/bin"
  case "$HOST_OS" in
    darwin)
      if [ -d "$build_bin/Spotter.app" ]; then
        rm -rf "$CLIENT_OUT/spotter-client-darwin-$HOST_ARCH.app"
        cp -R "$build_bin/Spotter.app" \
              "$CLIENT_OUT/spotter-client-darwin-$HOST_ARCH.app"
      fi
      if [ -f "$build_bin/spotter-client" ]; then
        install -m 0755 "$build_bin/spotter-client" \
          "$CLIENT_OUT/spotter-client-darwin-$HOST_ARCH"
      fi
      ;;
    windows)
      if [ -f "$build_bin/spotter-client.exe" ]; then
        install -m 0755 "$build_bin/spotter-client.exe" \
          "$CLIENT_OUT/spotter-client-windows-$HOST_ARCH.exe"
      fi
      ;;
    linux|*)
      if [ -f "$build_bin/spotter-client" ]; then
        install -m 0755 "$build_bin/spotter-client" \
          "$CLIENT_OUT/spotter-client-linux-$HOST_ARCH"
      fi
      ;;
  esac
}

# ---------- packaging ----------
copy_scripts() {
  log "scripts + deploy helpers"
  install -m 0755 "$PROJECT_ROOT/scripts/install.sh"   "$SCRIPT_OUT/install.sh"
  install -m 0755 "$PROJECT_ROOT/scripts/uninstall.sh" "$SCRIPT_OUT/uninstall.sh"
  install -m 0755 "$PROJECT_ROOT/scripts/cleanup.sh"   "$SCRIPT_OUT/cleanup.sh"
  install -m 0755 "$PROJECT_ROOT/scripts/deploy.sh"    "$SCRIPT_OUT/deploy.sh"
  install -m 0644 "$PROJECT_ROOT/scripts/deploy.ps1"   "$SCRIPT_OUT/deploy.ps1"
  install -m 0644 "$PROJECT_ROOT/scripts/spotterd.service" \
                  "$SCRIPT_OUT/spotterd.service"
  # Non-systemd init scripts for distros that don't ship it
  # (Alpine / Void / Artix). These live in their own subdirs so
  # an operator can grab the one matching their init system
  # without sifting through the systemd unit.
  install -m 0755 "$PROJECT_ROOT/scripts/init/openrc/spotterd" \
                  "$SCRIPT_OUT/init/openrc/spotterd"
  install -m 0755 "$PROJECT_ROOT/scripts/init/runit/spotterd.run" \
                  "$SCRIPT_OUT/init/runit/spotterd.run"
  ok "scripts + deploy helpers"
}

copy_readme() {
  log "README"
  cp "$PROJECT_ROOT/README.md" "$OUT_DIR/README.md"
  ok "README"
}

write_sha256sums() {
  log "SHA256SUMS"
  : > "$OUT_DIR/SHA256SUMS"
  while IFS= read -r -d '' f; do
    rel="${f#"$OUT_DIR"/}"
    printf '%s  %s\n' "$(sha256_file "$f")" "$rel" >> "$OUT_DIR/SHA256SUMS"
  done < <(find "$OUT_DIR" -type f -not -name SHA256SUMS -print0)
  ok "SHA256SUMS"
}

# ---------- main ----------
main() {
  log "Spotter release builder"
  echo "    host:    $HOST_OS/$HOST_ARCH"
  echo "    output:  $OUT_DIR"
  if [ -z "$WAILS" ]; then
    echo "    wails:   not found (will use 'go build' fallback for client)"
  else
    echo "    wails:   $WAILS"
  fi
  echo

  rm -rf "$OUT_DIR"
  mkdir -p "$AGENT_OUT" "$CLIENT_OUT" "$SCRIPT_OUT"

  # Agents: always build both Linux arches.
  build_agent arm64
  build_agent amd64

  if [ "$AGENT_ONLY" = true ]; then
    copy_scripts
    copy_readme
    write_sha256sums
    echo
    log "Done (agent-only). Output: $OUT_DIR"
    return 0
  fi

  # Client: native build for the current host only.
  build_client_host

  copy_scripts
  copy_readme
  write_sha256sums

  echo
  log "Done. Output: $OUT_DIR"
  echo
  echo "Contents:"
  (cd "$OUT_DIR" && find . -maxdepth 2 -mindepth 1 -print | sort)
}

main "$@"