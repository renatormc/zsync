#!/usr/bin/env bash
# zsync installer: downloads the latest release binary into ~/.local/bin
# and makes sure that folder is on the PATH via ~/.bashrc.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/renatormc/zsync/main/install.sh | bash
#
# Set ZSYNC_VERSION (e.g. v0.1.0) to install a specific version.

set -euo pipefail

REPO="renatormc/zsync"
BASE_URL="${ZSYNC_BASE_URL:-https://github.com/$REPO}"
INSTALL_DIR="$HOME/.local/bin"
BASHRC="$HOME/.bashrc"

info() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null || fail "curl is required"
command -v tar >/dev/null || fail "tar is required"

case "$(uname -s)" in
    Linux) os="unknown-linux-musl" ;;
    Darwin) os="apple-darwin" ;;
    *) fail "unsupported OS: $(uname -s). Download the binary manually from $BASE_URL/releases" ;;
esac

case "$(uname -m)" in
    x86_64 | amd64) arch="x86_64" ;;
    aarch64 | arm64) arch="aarch64" ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
esac

target="$arch-$os"

version="${ZSYNC_VERSION:-}"
if [ -z "$version" ]; then
    # /releases/latest redirects to /releases/tag/<version>
    latest_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$BASE_URL/releases/latest")" \
        || fail "could not determine the latest release"
    version="${latest_url##*/}"
    [ -n "$version" ] && [ "$version" != "latest" ] || fail "no release found at $BASE_URL/releases"
fi

name="zsync-$version-$target"
url="$BASE_URL/releases/download/$version/$name.tar.gz"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

info "Downloading zsync $version ($target)"
curl -fsSL "$url" -o "$tmp/zsync.tar.gz" || fail "download failed: $url"
tar xzf "$tmp/zsync.tar.gz" -C "$tmp"

mkdir -p "$INSTALL_DIR"
install -m 755 "$tmp/$name/zsync" "$INSTALL_DIR/zsync"
info "Installed to $INSTALL_DIR/zsync"

if ! grep -qs '\.local/bin' "$BASHRC"; then
    printf '\n# Added by zsync installer\nexport PATH="$HOME/.local/bin:$PATH"\n' >> "$BASHRC"
    info "Added $INSTALL_DIR to PATH in $BASHRC"
    info "Run 'source ~/.bashrc' or open a new terminal to use zsync"
fi

if ! command -v rclone >/dev/null; then
    info "Note: rclone was not found. Install it from https://rclone.org/install/"
fi

info "Done! Run 'zsync --help' to get started."
