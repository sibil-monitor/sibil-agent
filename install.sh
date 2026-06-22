#!/bin/sh
# Sibil Monitor CLI installer — read this before piping it into sh.
# Usage: curl -fsSL https://github.com/sibil-monitor/sibil-agent/releases/latest/download/install.sh | sh
#
# This does exactly what the manual install path in README.md does:
# download the binary, verify its checksum, install it. Nothing else.
set -e

REPO="sibil-monitor/sibil-agent"
VERSION="${SIBIL_VERSION:-latest}"
INSTALL_DIR="/usr/local/bin"
BINARY="sibil"

if [ "$VERSION" = "latest" ]; then
  BASE_URL="https://github.com/${REPO}/releases/latest/download"
else
  BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
fi

# ── Detect OS ────────────────────────────────────────────────────────────────
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$OS" != "linux" ]; then
  echo "Unsupported OS: $OS (Linux only for now)"
  exit 1
fi

# ── Detect architecture ───────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)          ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    echo "Download manually from: $BASE_URL"
    exit 1
    ;;
esac

FILENAME="${BINARY}-${OS}-${ARCH}"
BINARY_URL="${BASE_URL}/${FILENAME}"
CHECKSUMS_URL="${BASE_URL}/checksums.txt"

echo "Installing Sibil Monitor CLI (${VERSION}) for ${OS}/${ARCH}..."
echo ""

# ── Download binary + checksums ────────────────────────────────────────────────
TMP=$(mktemp)
TMPCHK=$(mktemp)
trap 'rm -f "$TMP" "$TMPCHK"' EXIT

if ! curl -fsSL "$BINARY_URL" -o "$TMP"; then
  echo "Download failed: $BINARY_URL"
  exit 1
fi
chmod +x "$TMP"

# ── Verify checksum (mandatory — abort if it can't be verified) ───────────────
if ! command -v sha256sum > /dev/null 2>&1; then
  echo "sha256sum not found — cannot verify the download. Aborting."
  echo "Install sha256sum, or verify manually against: $CHECKSUMS_URL"
  exit 1
fi

if ! curl -fsSL "$CHECKSUMS_URL" -o "$TMPCHK"; then
  echo "Could not fetch checksums.txt — aborting rather than installing unverified."
  exit 1
fi

# Anchored match: proves checksums.txt actually covers THIS exact filename,
# not just that some line happens to contain the substring.
if ! grep -q "  ${FILENAME}\$" "$TMPCHK"; then
  echo "No checksum entry for ${FILENAME} in checksums.txt — aborting."
  exit 1
fi

EXPECTED=$(grep "  ${FILENAME}\$" "$TMPCHK" | awk '{print $1}')
ACTUAL=$(sha256sum "$TMP" | awk '{print $1}')
if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Checksum mismatch — aborting."
  echo "  Expected : $EXPECTED"
  echo "  Got      : $ACTUAL"
  exit 1
fi
echo "  - Checksum verified for ${FILENAME}"

# ── Install ────────────────────────────────────────────────────────────────────
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "${INSTALL_DIR}/${BINARY}"
else
  echo "  Installing to ${INSTALL_DIR} (sudo required)"
  sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
fi
trap - EXIT
rm -f "$TMPCHK"

echo "  - Installed: ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Next steps:"
echo ""
echo "  sibil init        # generate config + token"
echo "  sibil doctor      # verify setup"
echo "  sibil start       # start the local API server"
echo ""
echo "Then open the Sibil Monitor app and enter your server URL + token."
