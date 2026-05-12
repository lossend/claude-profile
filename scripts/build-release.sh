#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${DIST_DIR:-$ROOT_DIR/dist}"
VERSION="${VERSION:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)}"
VERSION_TRIMMED="${VERSION#v}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo none)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"

TARGETS=(
  "darwin amd64"
  "darwin arm64"
  "linux amd64"
  "linux arm64"
  "windows amd64"
  "windows arm64"
)

mkdir -p "$DIST_DIR"
rm -f "$DIST_DIR"/claude-profile_* "$DIST_DIR"/claude-profile_checksums.txt

build_archive() {
  local goos="$1"
  local goarch="$2"
  local bin_name="claude-profile"
  local archive_name="claude-profile_${VERSION_TRIMMED}_${goos}_${goarch}"
  local stage_dir="$DIST_DIR/$archive_name"

  if [[ "$goos" == "windows" ]]; then
    bin_name="claude-profile.exe"
  fi

  rm -rf "$stage_dir"
  mkdir -p "$stage_dir"

  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.buildDate=$BUILD_DATE" \
      -o "$stage_dir/$bin_name" \
      "$ROOT_DIR"

  cp "$ROOT_DIR/README.md" "$stage_dir/README.md"

  if [[ "$goos" == "windows" ]]; then
    (
      cd "$DIST_DIR"
      zip -qr "${archive_name}.zip" "$archive_name"
    )
  else
    tar -C "$DIST_DIR" -czf "$DIST_DIR/${archive_name}.tar.gz" "$archive_name"
  fi

  rm -rf "$stage_dir"
}

checksum_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$@"
  else
    shasum -a 256 "$@"
  fi
}

for target in "${TARGETS[@]}"; do
  # shellcheck disable=SC2086
  build_archive $target
done

checksum_file "$DIST_DIR"/claude-profile_"${VERSION_TRIMMED}"_* > "$DIST_DIR/claude-profile_checksums.txt"
