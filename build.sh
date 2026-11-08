#!/bin/sh
# Builds, signs and packages one Coremon release track. Used by the GitHub
# Actions release workflows and for local builds on Linux/macOS.
#
#   ./build.sh agent 0.2.0              # agent binaries
#   ./build.sh master 0.2.0             # master package + installer
#   ARCHES=amd64 ./build.sh agent 0.2.0 # subset of architectures
#
# Output in dist/: assets/ (files to attach to the GitHub release),
# release/ (signed layout, input for `coremon-sign index`) and coremon-sign.
# Signing key: $COREMON_SIGNING_KEY (CI secret) or keys/signing.key.
# COREMON_REPO=owner/name bakes the GitHub repository into install-master.sh.
set -eu
cd "$(dirname "$0")"

TRACK="${1:-}"
VERSION="${2:-}"
ARCHES="${ARCHES:-amd64 arm64}"
REPO="${COREMON_REPO:-}"
case "$TRACK" in agent|master) ;; *) echo "usage: $0 agent|master VERSION" >&2; exit 2 ;; esac
echo "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "version must be X.Y.Z" >&2; exit 2; }

rm -rf dist
mkdir -p dist/release dist/assets

BUILD_DATE="$(date -u +%Y-%m-%d)"
gobuild() { # goarch pkg dst
  CGO_ENABLED=0 GOOS=linux GOARCH="$1" go build -trimpath -ldflags "-s -w -X main.version=$VERSION -X main.buildDate=$BUILD_DATE" -o "$3" "$2"
}

echo "Running tests..."
go vet ./...
go test ./...

if [ "$TRACK" = agent ]; then KIND=core; BIN=coremon-agent; PKG=./cmd/coremon-agent
else KIND=master; BIN=coremon-master; PKG=./cmd/coremon-master; fi

for A in $ARCHES; do
  echo "Building $BIN $VERSION linux/$A ..."
  gobuild "$A" "$PKG" "dist/release/$KIND/$VERSION/linux-$A/$BIN"
done

go build -o dist/coremon-sign ./cmd/coremon-sign
if [ -z "${COREMON_SIGNING_KEY:-}" ] && [ ! -f keys/signing.key ]; then
  [ -z "${CI:-}" ] || { echo "COREMON_SIGNING_KEY is not set" >&2; exit 1; }
  echo "Creating local signing key in keys/ ..."
  dist/coremon-sign keygen -key keys/signing.key -pub keys/signing.pub
fi
dist/coremon-sign release -key keys/signing.key -dir dist/release

# Raw binaries + signatures as release assets (downloaded by the master
# via the version index).
for A in $ARCHES; do
  cp "dist/release/$KIND/$VERSION/linux-$A/$BIN" "dist/assets/$BIN-$VERSION-linux-$A"
  cp "dist/release/$KIND/$VERSION/linux-$A/$BIN.sig" "dist/assets/$BIN-$VERSION-linux-$A.sig"
done

if [ "$TRACK" = master ]; then
  NAME="coremon-master-$VERSION"
  PK="dist/$NAME"
  mkdir -p "$PK/release"
  cp -R dist/release/master "$PK/release/"
  cp dist/release/signing.pub "$PK/release/"
  cp deploy/coremon-master.service deploy/coremon-master-update.service deploy/coremon-master-update.path "$PK/"
  subst() { sed -e "s|@REPO@|$REPO|g" -e "s|@VERSION@|$VERSION|g" -e "s|@SHA256@|$2|g" deploy/install-master.sh > "$1"; }
  subst "$PK/install-master.sh" ""
  tar -C dist -czf "dist/assets/$NAME.tar.gz" "$NAME"
  SHA="$(sha256sum "dist/assets/$NAME.tar.gz" | cut -d' ' -f1)"
  subst dist/assets/install-master.sh "$SHA"
fi

(cd dist/assets && sha256sum $(ls | grep -v '\.sig$') > SHA256SUMS)
echo
echo "Release assets ready in dist/assets:"
ls -l dist/assets
