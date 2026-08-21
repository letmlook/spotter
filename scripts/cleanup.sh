#!/usr/bin/env bash
# Cleanup is best-effort: each step is independent so we leave the
# system in the cleanest state we can if install.sh failed mid-way.
set +e

CONFIG_DIR="${CONFIG_DIR:-/etc/spotterd}"
AGENT_DST="${AGENT_DST:-/usr/local/bin/spotterd}"
UNIT_DST="${UNIT_DST:-/etc/systemd/system/spotterd.service}"

systemctl stop spotterd 2>/dev/null
systemctl disable spotterd 2>/dev/null

rm -f "$UNIT_DST"
rm -f "$AGENT_DST"
rm -rf "$CONFIG_DIR"

systemctl daemon-reload 2>/dev/null

# Remove /tmp scratch files
rm -f /tmp/spotterd /tmp/install.sh /tmp/spotterd.service

echo "cleanup: best-effort done"