#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE="${REMOTE:-origin}"
DEFAULT_BRANCH="${DEFAULT_BRANCH:-main}"
DRY_RUN=0

usage() {
  cat <<'EOF'
Usage: scripts/release.sh [--dry-run] vX.Y.Z

Runs the full release flow:
1. verify you are on main with a clean worktree
2. run go test ./...
3. create the release tag
4. push main
5. push the tag

Options:
  --dry-run   print the commands without mutating git state
EOF
}

run_cmd() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf '[dry-run] %q' "$1"
    shift
    for arg in "$@"; do
      printf ' %q' "$arg"
    done
    printf '\n'
    return 0
  fi

  "$@"
}

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage >&2
  exit 1
fi

if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
  shift
fi

if [[ $# -ne 1 ]]; then
  usage >&2
  exit 1
fi

VERSION="$1"

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "version must match vX.Y.Z" >&2
  exit 1
fi

CURRENT_BRANCH="$(git -C "$ROOT_DIR" branch --show-current)"
if [[ "$CURRENT_BRANCH" != "$DEFAULT_BRANCH" ]]; then
  echo "release must run from $DEFAULT_BRANCH; current branch is $CURRENT_BRANCH" >&2
  exit 1
fi

if [[ -n "$(git -C "$ROOT_DIR" status --short)" ]]; then
  echo "worktree must be clean before release" >&2
  exit 1
fi

if git -C "$ROOT_DIR" rev-parse "$VERSION" >/dev/null 2>&1; then
  echo "tag $VERSION already exists" >&2
  exit 1
fi

echo "Running tests before release..."
run_cmd bash -lc "cd \"$ROOT_DIR\" && go test ./..."

echo "Creating tag $VERSION..."
run_cmd git -C "$ROOT_DIR" tag "$VERSION"

echo "Pushing $DEFAULT_BRANCH to $REMOTE..."
run_cmd git -C "$ROOT_DIR" push "$REMOTE" "$DEFAULT_BRANCH"

echo "Pushing tag $VERSION to $REMOTE..."
run_cmd git -C "$ROOT_DIR" push "$REMOTE" "$VERSION"

echo "Release trigger complete for $VERSION"
