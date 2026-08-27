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

# Generate a per-device bearer token when auth is enabled (the
# default since v0.3). The token is saved to agent.toml with
# 0600 permissions and echoed on stdout so the GUI client can
# prompt the user to copy it into Settings → Auth tokens.
AUTH_TOKEN="${AUTH_TOKEN:-}"
if [[ -z "$AUTH_TOKEN" ]]; then
  if [[ -r /proc/sys/kernel/random/uuid ]]; then
    AUTH_TOKEN="$(cat /proc/sys/kernel/random/uuid)"
  else
    AUTH_TOKEN="$(awk 'BEGIN { srand(); printf "spotter-%08x-%04x-%04x-%04x-%012x\n", \
        rand()*4294967295, rand()*65535, rand()*65535, \
        rand()*65535, rand()*281474976710655 }')"
  fi
fi

cat >"$CONFIG_DIR/agent.toml" <<EOF
device_id = "$DEVICE_ID"
listen_addr = "0.0.0.0:9999"
multicast_group = "239.255.42.42:9999"
agent_version = "${SPOTTER_AGENT_VERSION:-0.1.0}"
enable_power_actions = true
enable_log_stream = true
log_unit = "spotterd.service"
hello_interval = "5s"

[auth]
enabled = true
token = "$AUTH_TOKEN"
EOF

chmod 0600 "$CONFIG_DIR/agent.toml"

install -m 0644 "$UNIT_SRC" "$UNIT_DST"
systemctl daemon-reload
systemctl enable --now spotterd

# If the host is not a systemd distro (Alpine, Void, Artix),
# print a hint pointing at the right init script under
# scripts/init/. The user runs that manually; install.sh does
# not auto-install OpenRC/runit because the path layout differs
# per distro and we'd rather not guess.
if [ ! -d /run/systemd/system ] && [ "$(command -v openrc-run 2>/dev/null)" != "" ]; then
  echo
  echo "Detected OpenRC host; install with:"
  echo "  install -m 0755 $UNIT_SRC /etc/init.d/spotterd"
  echo "  rc-update add spotterd default"
  echo "  rc-service spotterd start"
elif [ ! -d /run/systemd/system ] && [ -d /etc/sv ] && [ "$(command -v sv 2>/dev/null)" != "" ]; then
  echo
  echo "Detected runit host (Void/Artix); install with:"
  echo "  install -m 0755 scripts/init/runit/spotterd.run /etc/sv/spotterd/run"
  echo "  ln -s /etc/sv/spotterd /var/service/"
  echo "  sv start spotterd"
fi

# Allow time for service to start, then report status.
sleep 1
if ! systemctl is-active --quiet spotterd; then
  echo "install: spotterd failed to start" >&2
  systemctl status spotterd || true
  exit 1
fi

echo "DEVICE_ID=$DEVICE_ID"
echo "AUTH_TOKEN=$AUTH_TOKEN"
echo
echo "✔ Saved $CONFIG_DIR/agent.toml (mode 0600, owned by root:root)."
echo "✔ spotterd.service is active."
echo
echo "Next: paste the AUTH_TOKEN above into the Spotter client"
echo "      Settings → Auth tokens. The token cannot be retrieved"
echo "      later from the agent."