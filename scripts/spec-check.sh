#!/usr/bin/env bash
# spec-check.sh — CI helper that catches drift between docs specs and
# code defaults. See docs/superpowers/SPEC_REVIEW.md for the policy.
#
# Exit code 0 = no drift, 1 = drift detected.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

DEVIATIONS_FILE="docs/superpowers/SPEC_DEVIATIONS.md"

if [ ! -f "$DEVIATIONS_FILE" ]; then
    echo "spec-check: $DEVIATIONS_FILE missing — see docs/superpowers/SPEC_REVIEW.md"
    exit 1
fi

EXIT=0

echo "== spec-check: scanning defaults in production code =="

# Build a tempfile of code default annotations. We look for *comments*
# declaring a default value, not bare literal assignments; the latter
# would flag every time.Tick(5*time.Second) across the tree.
TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT
grep -nrE '//.*默认\s*(true|false|关|开|off|on)' \
    internal/ cmd/ main.go 2>/dev/null \
    | grep -v _test.go > "$TMP" || true

# Also flag hardcoded listener ports whose value isn't covered by a
# spec; these tend to drift when the spec bumps a default but the
# code constant is left untouched.
grep -nrE 'const\s+listenPort\s*=\s*9999' \
    internal/ cmd/ main.go 2>/dev/null \
    | grep -v _test.go >> "$TMP" || true

if [ ! -s "$TMP" ]; then
    echo "  (no candidates)"
else
    while IFS=: read -r file lineno text rest; do
        # Look for the same text (or its field token) in specs/ or DEVIATIONS.
        if grep -qF "$text" docs/superpowers/specs/ -r 2>/dev/null; then
            continue
        fi
        # Pull a distinctive token out of the line for matching against
        # the DEVIATIONS log — usually the field or constant name.
        token=$(echo "$text" | grep -oE 'enable_[a-z_]+|[A-Z][a-zA-Z]+' | head -1 || true)
        if [ -n "$token" ] && grep -qF "$token" "$DEVIATIONS_FILE" 2>/dev/null; then
            continue
        fi
        echo "  drift candidate: $file:$lineno  $text"
        EXIT=1
    done < "$TMP"
fi

if [ "$EXIT" -ne 0 ]; then
    echo
    echo "spec-check: drift detected."
    echo "  Either update the relevant spec file, or add an entry to"
    echo "  $DEVIATIONS_FILE documenting the deviation. See SPEC_REVIEW.md."
fi

exit "$EXIT"
