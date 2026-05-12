#!/usr/bin/env sh
set -eu

REPO="${REPO:-lossend/claude-profile}"
BINDIR="${BINDIR:-$HOME/.local/bin}"
VERSION="${VERSION:-}"
TMPDIR="${TMPDIR:-/tmp}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need_cmd uname
need_cmd tar

if command -v curl >/dev/null 2>&1; then
  fetch() {
    curl -fsSL "$1"
  }
elif command -v wget >/dev/null 2>&1; then
  fetch() {
    wget -qO- "$1"
  }
else
  echo "missing required command: curl or wget" >&2
  exit 1
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  darwin|linux) ;;
  *)
    echo "unsupported OS: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

if [ -z "$VERSION" ]; then
  VERSION="$(fetch "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
fi

if [ -z "$VERSION" ]; then
  echo "failed to determine release version" >&2
  exit 1
fi

version_trimmed="${VERSION#v}"
archive="claude-profile_${version_trimmed}_${os}_${arch}.tar.gz"
download_url="https://github.com/$REPO/releases/download/$VERSION/$archive"
tmp_archive="$TMPDIR/$archive"

mkdir -p "$BINDIR"
fetch "$download_url" > "$tmp_archive"
tar -xzf "$tmp_archive" -C "$TMPDIR"
install -m 0755 "$TMPDIR/claude-profile_${version_trimmed}_${os}_${arch}/claude-profile" "$BINDIR/claude-profile"
rm -rf "$tmp_archive" "$TMPDIR/claude-profile_${version_trimmed}_${os}_${arch}"

echo "installed claude-profile $VERSION to $BINDIR/claude-profile"
echo "make sure $BINDIR is on your PATH"
