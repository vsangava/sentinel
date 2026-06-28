#!/usr/bin/env bash
set -euo pipefail

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This installer only supports macOS. Download the Windows binary from:"
  echo "https://github.com/vsangava/sentinel/releases/latest"
  exit 1
fi

ARCH=$(uname -m)
case "$ARCH" in
  arm64)  BINARY="sentinel-macos-arm64" ;;
  x86_64) BINARY="sentinel-macos-amd64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Downloading $BINARY..."
TMP=$(mktemp)
# GitHub's /releases/latest/download/<asset> redirects to the latest release's
# asset URL — no API call, no JSON parsing, no rate-limit concerns. Use -fSL
# (not -fsSL) so transfer errors and progress are visible if anything fails.
curl -fSL -o "$TMP" "https://github.com/vsangava/sentinel/releases/latest/download/$BINARY"

# Remove Gatekeeper quarantine flag (no-op if not set).
xattr -d com.apple.quarantine "$TMP" 2>/dev/null || true

chmod +x "$TMP"

echo "Installing..."
sudo "$TMP" setup

rm -f "$TMP"
