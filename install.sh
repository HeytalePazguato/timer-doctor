#!/bin/sh
# install.sh — fetch the latest timer-doctor release for this OS/arch
# from GitHub and drop the binary into /usr/local/bin (or ~/.local/bin
# when the user lacks root). POSIX sh; works on Linux and macOS.

set -eu

REPO="HeytalePazguato/timer-doctor"
BINARY="timer-doctor"
INSTALL_DIR="${TIMER_DOCTOR_INSTALL_DIR:-}"

err() { echo "install.sh: $*" >&2; exit 1; }

uname_s=$(uname -s 2>/dev/null || echo unknown)
uname_m=$(uname -m 2>/dev/null || echo unknown)

case "$uname_s" in
  Linux*)  os=linux ;;
  Darwin*) os=darwin ;;
  *) err "unsupported OS: $uname_s. Download manually from https://github.com/$REPO/releases" ;;
esac

case "$uname_m" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported arch: $uname_m" ;;
esac

if [ -z "$INSTALL_DIR" ]; then
  if [ -w /usr/local/bin ] 2>/dev/null; then
    INSTALL_DIR=/usr/local/bin
  else
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
  fi
fi

# Resolve the latest release tag via the GitHub API redirect.
LATEST_URL="https://api.github.com/repos/$REPO/releases/latest"
if command -v curl >/dev/null 2>&1; then
  TAG=$(curl -sSL "$LATEST_URL" | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p' | head -n1)
elif command -v wget >/dev/null 2>&1; then
  TAG=$(wget -qO- "$LATEST_URL" | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p' | head -n1)
else
  err "neither curl nor wget is installed"
fi

[ -n "$TAG" ] || err "could not resolve latest release tag"

ARCHIVE="${BINARY}_${os}_${arch}.tar.gz"
URL="https://github.com/$REPO/releases/download/$TAG/$ARCHIVE"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $URL"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$tmp/$ARCHIVE"
else
  wget -qO "$tmp/$ARCHIVE" "$URL"
fi

tar -C "$tmp" -xzf "$tmp/$ARCHIVE"
install -m 0755 "$tmp/$BINARY" "$INSTALL_DIR/$BINARY"

echo "Installed $BINARY $TAG to $INSTALL_DIR/$BINARY"
"$INSTALL_DIR/$BINARY" --version || true

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Note: $INSTALL_DIR is not on your PATH. Add it to use the command anywhere." ;;
esac
