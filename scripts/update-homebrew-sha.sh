#!/usr/bin/env bash
# update-homebrew-sha.sh — fetch the latest tarball's sha256 from
# GitHub Releases and emit a one-line `sha256 "<value>"` ready to
# drop into scripts/packages/homebrew/spotter.rb. Run from CI when
# cutting a release tag.
#
# Usage: GITHUB_REPO=spotter/spotter VERSION=v1.0.0 ./scripts/update-homebrew-sha.sh
#
# When neither GITHUB_REPO nor VERSION is set, defaults to the
# repository's most recent tagged release.

set -euo pipefail

REPO="${GITHUB_REPO:-spotter/spotter}"
TAG="${VERSION:-}"
if [ -z "$TAG" ]; then
  TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' | head -1)"
fi
URL="https://github.com/${REPO}/archive/refs/tags/${TAG}.tar.gz"
echo "Computing sha256 for $URL" >&2

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT
if command -v sha256sum >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$TMP"
  sha=$(sha256sum "$TMP" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$TMP"
  sha=$(shasum -a 256 "$TMP" | awk '{print $1}')
else
  echo "neither sha256sum nor shasum found" >&2
  exit 1
fi

echo "Replace the sha256 in scripts/packages/homebrew/spotter.rb with:"
echo "  sha256 \"$sha\""
