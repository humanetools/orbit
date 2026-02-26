#!/bin/sh
set -e

REPO="humanetools/orbit"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)              echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest version
VERSION=$(curl -sI "https://github.com/$REPO/releases/latest" | grep -i "^location:" | sed 's|.*/v||' | tr -d '\r\n')
if [ -z "$VERSION" ]; then
  echo "Failed to fetch latest version"
  exit 1
fi

FILENAME="orbit_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/v${VERSION}/${FILENAME}"

echo "Installing orbit v${VERSION} (${OS}/${ARCH})..."

# Create install directory
mkdir -p "$INSTALL_DIR"

# Download and extract
TMP=$(mktemp -d)
curl -sL "$URL" -o "$TMP/$FILENAME"
tar xzf "$TMP/$FILENAME" -C "$TMP"
mv "$TMP/orbit" "$INSTALL_DIR/orbit"
chmod +x "$INSTALL_DIR/orbit"
rm -rf "$TMP"

# Check if INSTALL_DIR is in PATH
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    SHELL_NAME=$(basename "$SHELL")
    case "$SHELL_NAME" in
      zsh)  RC="$HOME/.zshrc" ;;
      *)    RC="$HOME/.bashrc" ;;
    esac
    echo "export PATH=\"$INSTALL_DIR:\$PATH\"" >> "$RC"
    echo "Added $INSTALL_DIR to PATH in $RC"
    echo "Run 'source $RC' or open a new terminal to use orbit."
    ;;
esac

echo "orbit v${VERSION} installed to $INSTALL_DIR/orbit"
"$INSTALL_DIR/orbit" --version
