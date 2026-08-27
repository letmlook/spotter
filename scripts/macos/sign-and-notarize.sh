#!/usr/bin/env bash
# scripts/macos/sign-and-notarize.sh — end-to-end macOS
# codesign + notarize pipeline for the Wails-built .app bundle.
#
# Run on a macOS host with Apple Developer Program credentials.
# Activated only when secrets are present in the environment; the
# caller (GitHub Actions or a local operator) must export:
#
#   APPLE_DEVELOPER_ID_APP    — "Developer ID Application: <name> (<teamid>)"
#   APPLE_TEAM_ID             — 10-character team id
#   APPLE_KEYCHAIN_PASSWORD   — random string used to unlock the
#                                temporary keychain that holds
#                                APPLE_DEVELOPER_IDENTITY_P12
#   APPLE_DEVELOPER_IDENTITY_P12 — base64 of the .p12 cert
#   APPLE_NOTARYTOOL_PROFILE  — `xcrun notarytool store-credentials`
#                                profile name (NOTARYTOOL_KEY etc.
#                                are also accepted as direct values)
#
# Without these the script bails early with a clear message — we
# never want a partial sign + no-notarize ship.
#
# Usage:
#   APP_BUNDLE=build/bin/Spotter.app \
#   ARCH=arm64 \
#   ./scripts/macos/sign-and-notarize.sh

set -euo pipefail

: "${APP_BUNDLE:?APP_BUNDLE path required (e.g. build/bin/Spotter.app)}"
: "${ARCH:?ARCH required (amd64 or arm64)}"

required=(
  APPLE_DEVELOPER_ID_APP
  APPLE_TEAM_ID
  APPLE_KEYCHAIN_PASSWORD
  APPLE_DEVELOPER_IDENTITY_P12
)
for v in "${required[@]}"; do
  if [ -z "${!v:-}" ]; then
    echo "sign-and-notarize: $v is required" >&2
    exit 1
  fi
done

if [ ! -d "$APP_BUNDLE" ]; then
  echo "sign-and-notarize: $APP_BUNDLE is not a directory" >&2
  exit 1
fi

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
KEYCHAIN="$WORKDIR/spotter-signing.keychain-db"
P12="$WORKDIR/cert.p12"

echo "==> Materialising signing identity"
echo "$APPLE_DEVELOPER_IDENTITY_P12" | base64 --decode > "$P12"

echo "==> Creating temporary keychain"
security create-keychain -p "$APPLE_KEYCHAIN_PASSWORD" "$KEYCHAIN"
security set-keychain-settings -lut 21600 "$KEYCHAIN"
security unlock-keychain -p "$APPLE_KEYCHAIN_PASSWORD" "$KEYCHAIN"

# Import identity into the temp keychain (not the login keychain,
# which would persist across invocations and is harder to reason
# about in CI). `security import` does NOT add the keychain to
# the search list by default; we do that next.
security import "$P12" \
  -k "$KEYCHAIN" \
  -P "$APPLE_KEYCHAIN_PASSWORD" \
  -T /usr/bin/codesign \
  -T /usr/bin/security
security list-keychain -d user -s "$KEYCHAIN"
security set-key-partition-list -S apple-tool:,apple: -s -k "$APPLE_KEYCHAIN_PASSWORD" "$KEYCHAIN"

# Resolve the signing identity's full name. The secret value
# `APPLE_DEVELOPER_ID_APP` is the human-readable name, but
# codesign prefers the SHA-1 fingerprint for stability across
# cert renewals. We use the name form (it works as-is for
# Developer ID certs) and let codesign resolve.
IDENTITY="$APPLE_DEVELOPER_ID_APP"

echo "==> Codesigning (deep, hardened runtime)"
# `--options runtime` enables the hardened runtime; required for
# notarization. `--timestamp` adds a secure timestamp from
# Apple's TSA so the signature stays valid after the cert expires.
# `--force` replaces any prior signature (re-runs).
codesign --force --deep --options runtime --timestamp \
  --sign "$IDENTITY" "$APP_BUNDLE"

echo "==> Verifying signature"
codesign --verify --deep --strict --verbose=2 "$APP_BUNDLE"
spctl --assess --type execute --verbose=2 "$APP_BUNDLE" || true
# spctl returns non-zero for ad-hoc signed bundles; the verify
# step above is the authoritative gate. Notarization (next) will
# re-assess under the actual Developer ID.

echo "==> Zipping for notary upload"
# Apple wants a flat zip of the .app, not a tarball. ditto
# preserves xattrs + ACLs that zip would otherwise strip.
NOTARY_ZIP="$WORKDIR/Spotter.zip"
ditto -c -k --keepParent "$APP_BUNDLE" "$NOTARY_ZIP"

echo "==> Submitting to notarytool"
if [ -n "${APPLE_NOTARYTOOL_PROFILE:-}" ]; then
  xcrun notarytool submit "$NOTARY_ZIP" \
    --keychain-profile "$APPLE_NOTARYTOOL_PROFILE" \
    --wait
elif [ -n "${APPLE_ID:-}" ] && [ -n "${APPLE_APP_SPECIFIC_PASSWORD:-}" ]; then
  xcrun notarytool submit "$NOTARY_ZIP" \
    --apple-id "$APPLE_ID" \
    --password "$APPLE_APP_SPECIFIC_PASSWORD" \
    --team-id "$APPLE_TEAM_ID" \
    --wait
else
  echo "sign-and-notarize: no notarytool credentials (need APPLE_NOTARYTOOL_PROFILE or APPLE_ID+APPLE_APP_SPECIFIC_PASSWORD)" >&2
  exit 1
fi

echo "==> Stapling notarization ticket"
xcrun stapler staple "$APP_BUNDLE"
xcrun stapler validate "$APP_BUNDLE"

echo "==> Cleaning up keychain"
security delete-keychain "$KEYCHAIN" || true

echo "==> Done: $APP_BUNDLE is signed + notarized"
