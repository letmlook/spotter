#!/usr/bin/env bash
set -euo pipefail

CONFIG_DIR="${CONFIG_DIR:-/etc/spotterd}"
AGENT_DST="${AGENT_DST:-/usr/local/bin/spotterd}"
UNIT_DST="${UNIT_DST:-/etc/systemd/system/spotterd.service}"

if systemctl is-active --quiet spotterd; then
  systemctl stop spotterd
fi
if systemctl is-enabled --quiet spotterd; then
  systemctl disable spotterd
fi

rm -f "$UNIT_DST"
rm -f "$AGENT_DST"
rm -rf "$CONFIG_DIR"
systemctl daemon-reload
echo "uninstall: ok"