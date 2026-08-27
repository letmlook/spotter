#!/usr/bin/env bash
# scripts/macos/make-dmg.sh — package a signed+notarized Spotter.app
# into a downloadable DMG.
#
# Dependencies (auto-installed via brew if missing):
#   - create-dmg (https://github.com/create-dmg/create-dmg)
#
# Usage:
#   APP_BUNDLE=build/bin/Spotter.app \
#   ARCH=arm64 \
#   OUTPUT_DIR=dist/clients \
#   ./scripts/macos/make-dmg.sh

set -euo pipefail

: "${APP_BUNDLE:?APP_BUNDLE path required (e.g. build/bin/Spotter.app)}"
: "${ARCH:?ARCH required (amd64 or arm64)}"
: "${OUTPUT_DIR:=dist/clients}"

VERSION="${VERSION:-$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$APP_BUNDLE/Contents/Info.plist" 2>/dev/null || echo dev)}"
DMG_NAME="Spotter-${VERSION}-darwin-${ARCH}.dmg"
DMG_PATH="$OUTPUT_DIR/$DMG_NAME"

if ! command -v create-dmg >/dev/null 2>&1; then
  echo "make-dmg: create-dmg not found; install with: brew install create-dmg" >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

# --no-internet-enable is intentional: the DMG is signed offline,
# and Gatekeeper verifies against the embedded stapled ticket
# (not against a live notary check). Disabling the internet
# enable bit also speeds up first launch on slow connections.
create-dmg \
  --no-internet-enable \
  --volname "Spotter $VERSION" \
  --window-size 540 380 \
  --icon-size 96 \
  --icon "Spotter.app" 130 180 \
  --hide-extension "Spotter.app" \
  --app-drop-link 410 180 \
  --codesign "$APPLE_DEVELOPER_ID_APP" \
  "$DMG_PATH" \
  "$APP_BUNDLE"

echo "make-dmg: $DMG_PATH"
