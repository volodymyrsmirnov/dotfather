#!/bin/bash
set -euo pipefail

REPO="volodymyrsmirnov/dotfather"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="dotfather"

# Determine version
VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
fi

if [ -z "$VERSION" ]; then
  echo "Error: Could not determine latest version" >&2
  exit 1
fi

# Determine OS and architecture
OS=$(uname -s)
ARCH=$(uname -m)

case "${OS}" in
  Darwin)
    ARTIFACT="dotfather-osx-universal"
    ;;
  Linux)
    case "${ARCH}" in
      x86_64|amd64)
        ARTIFACT="dotfather-linux-amd64"
        ;;
      aarch64|arm64)
        ARTIFACT="dotfather-linux-arm64"
        ;;
      *)
        echo "Error: Unsupported architecture: ${ARCH}" >&2
        exit 1
        ;;
    esac
    ;;
  *)
    echo "Error: Unsupported OS: ${OS}" >&2
    exit 1
    ;;
esac

URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARTIFACT}"

echo "Installing ${BINARY_NAME} ${VERSION} (${OS}/${ARCH})..."
echo "Downloading ${URL}"

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

curl -fsSL -o "$TMP" "$URL"
chmod +x "$TMP"

# Remove macOS quarantine attribute
if [ "$OS" = "Darwin" ]; then
  xattr -d com.apple.quarantine "$TMP" 2>/dev/null || true
fi

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "${INSTALL_DIR}/${BINARY_NAME}"
else
  sudo mv "$TMP" "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
"${INSTALL_DIR}/${BINARY_NAME}" --version
