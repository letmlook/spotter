#!/usr/bin/env bash
set -euo pipefail

# spotterd installer. Invoked by the GUI client over SSH:
#   SPOTTER_AGENT_VERSION=<ver> bash /tmp/install.sh
# Reads the agent binary from /tmp/spotterd and unit from /tmp/spotterd.service.
# Works on any systemd-equipped Linux: the binary is uploaded by the
# client with the correct GOARCH (arm64 or amd64) chosen at deploy time.

AGENT_SRC="${AGENT_SRC:-/tmp/spotterd}"
UNIT_SRC="${UNIT_SRC:-/tmp/spotterd.service}"
AGENT_DST="${AGENT_DST:-/usr/local/bin/spotterd}"
UNIT_DST="${UNIT_DST:-/etc/systemd/system/spotterd.service}"
CONFIG_DIR="${CONFIG_DIR:-/etc/spotterd}"

if [[ ! -f "$AGENT_SRC" ]]; then
  echo "install: missing agent binary at $AGENT_SRC" >&2
  exit 1
fi
if [[ ! -f "$UNIT_SRC" ]]; then
  echo "install: missing unit file at $UNIT_SRC" >&2
  exit 1
fi

install -m 0755 "$AGENT_SRC" "$AGENT_DST"
mkdir -p "$CONFIG_DIR"

DEVICE_ID="${DEVICE_ID:-$(cat /proc/sys/kernel/random/uuid)}"

cat >"$CONFIG_DIR/agent.toml" <<EOF
device_id = "$DEVICE_ID"
listen_addr = "0.0.0.0:9999"
multicast_group = "239.255.42.42:9999"
agent_version = "${SPOTTER_AGENT_VERSION:-0.1.0}"
EOF

install -m 0644 "$UNIT_SRC" "$UNIT_DST"
systemctl daemon-reload
systemctl enable --now spotterd

# Allow time for service to start, then report status.
sleep 1
if ! systemctl is-active --quiet spotterd; then
  echo "install: spotterd failed to start" >&2
  systemctl status spotterd || true
  exit 1
fi

echo "DEVICE_ID=$DEVICE_ID"