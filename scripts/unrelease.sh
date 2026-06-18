#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_NAME="${APP_NAME:-uvoo-minicms}"
REMOTE="${REMOTE:-origin}"

if [ -z "${VERSION:-}" ]; then
  echo "usage: VERSION=v0.1.0 make unrelease" >&2
  exit 2
fi

TAG="$VERSION"
case "$TAG" in
  v*) ;;
  *) TAG="v$TAG" ;;
esac

case "$TAG" in
  *[!A-Za-z0-9._-]*)
    echo "invalid release tag: $TAG" >&2
    exit 2
    ;;
esac

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

need git
need gh

cd "$ROOT"

gh auth status >/dev/null

echo "removing GitHub release and tag for $APP_NAME $TAG"
if gh release view "$TAG" >/dev/null 2>&1; then
  gh release delete "$TAG" --yes --cleanup-tag
else
  echo "GitHub release not found: $TAG"
fi

set +e
git ls-remote --exit-code --tags "$REMOTE" "refs/tags/$TAG" >/dev/null
remote_tag_status=$?
set -e
if [ "$remote_tag_status" -eq 0 ]; then
  echo "deleting remote tag $TAG from $REMOTE"
  git push "$REMOTE" ":refs/tags/$TAG"
elif [ "$remote_tag_status" -ne 2 ]; then
  echo "could not check tags on remote $REMOTE" >&2
  exit 1
else
  echo "remote tag not found on $REMOTE: $TAG"
fi

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
  echo "deleting local tag $TAG"
  git tag -d "$TAG" >/dev/null
else
  echo "local tag not found: $TAG"
fi

echo "removed release $TAG; rerun with: VERSION=$TAG make release"
