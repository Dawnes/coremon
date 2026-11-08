#!/bin/sh
# Adds a just-built release to the signed version index (versions.json in
# the GitHub release "index") and uploads the new index. Run by the release
# workflows after build.sh; needs GH_TOKEN, COREMON_SIGNING_KEY and
# GITHUB_REPOSITORY / GITHUB_REF_NAME from the Actions environment.
#
#   sh deploy/update-index.sh agent 0.2.1
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
# Start from the published index. Only start empty if there is none yet;
# if it exists but cannot be downloaded, fail rather than lose entries.
if gh release view index --repo "$REPO" --json assets --jq '.assets[].name' | grep -qx versions.json; then
  gh release download index --repo "$REPO" -p versions.json -p versions.json.sig -D "$D" --clobber
fi

dist/coremon-sign index -track "$TRACK" -version "$VERSION" -dir dist/release \
  -base "https://github.com/$REPO/releases/download/$TAG" \
  -in "$D/versions.json" -out "$D/versions.json"

gh release upload index --repo "$REPO" "$D/versions.json" "$D/versions.json.sig" --clobber
echo "Version index updated:"
cat "$D/versions.json"
