#!/bin/sh
# Adds a built release to the signed version index (run by the release workflows).

set -eu
TRACK="$1"
VERSION="$2"
REPO="$GITHUB_REPOSITORY"
TAG="$GITHUB_REF_NAME"
D=dist/index
mkdir -p "$D"

gh release view index --repo "$REPO" >/dev/null 2>&1 || {
  echo "The release 'index' does not exist. Create it once (see README, One-time GitHub setup)." >&2
  exit 1
}
if gh release view index --repo "$REPO" --json assets --jq '.assets[].name' | grep -qx versions.json; then
  gh release download index --repo "$REPO" -p versions.json -p versions.json.sig -D "$D" --clobber
fi

dist/coremon-sign index -track "$TRACK" -version "$VERSION" -dir dist/release \
  -base "https://github.com/$REPO/releases/download/$TAG" \
  -in "$D/versions.json" -out "$D/versions.json"

gh release upload index --repo "$REPO" "$D/versions.json" "$D/versions.json.sig" --clobber
echo "Version index updated:"
cat "$D/versions.json"
