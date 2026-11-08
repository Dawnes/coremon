#!/bin/sh
# Builds, signs and packages a Coremon release. Used by the GitHub Actions
# release workflow and for local builds on Linux/macOS.
#
#   ./build.sh                          # version from VERSION file
#   VERSION=1.2.3 ./build.sh            # explicit version (CI: from the git tag)
#   ARCHES=amd64 ./build.sh             # subset of architectures
#
# Signing key: $COREMON_SIGNING_KEY (CI secret) or keys/signing.key.
# COREMON_REPO=owner/name bakes the GitHub repository into install-master.sh.
set -eu
cd "$(dirname "$0")"

VERSION="${VERSION:-$(tr -d '[:space:]' < VERSION)}"
ARCHES="${ARCHES:-amd64 arm64}"
REPO="${COREMON_REPO:-}"
NAME="coremon-$VERSION"
OUT="dist/$NAME"
rm -rf dist
mkdir -p "$OUT/release"

gobuild() { # goarch pkg dst version
  CGO_ENABLED=0 GOOS=linux GOARCH="$1" go build -trimpath -ldflags "-s -w -X main.version=$4" -o "$3" "$2"
}

echo "Running tests..."
go vet ./...
go test ./...

for A in $ARCHES; do
  echo "Building linux/$A ..."
  gobuild "$A" ./cmd/coremon-master "$OUT/release/master/$VERSION/linux-$A/coremon-master" "$VERSION"
  gobuild "$A" ./cmd/coremon-agent "$OUT/release/core/$VERSION/linux-$A/coremon-agent" "$VERSION"
  for M in modules/*/; do
    M="$(basename "$M")"
    MV="$(tr -d '[:space:]' < "modules/$M/VERSION")"
    gobuild "$A" "./modules/$M" "$OUT/release/modules/$M/$MV/linux-$A/$M" "$MV"
  done
done

go build -o dist/coremon-sign ./cmd/coremon-sign
if [ -z "${COREMON_SIGNING_KEY:-}" ] && [ ! -f keys/signing.key ]; then
  [ -z "${CI:-}" ] || { echo "COREMON_SIGNING_KEY is not set" >&2; exit 1; }
  echo "Creating local signing key in keys/ ..."
  dist/coremon-sign keygen -key keys/signing.key -pub keys/signing.pub
fi
dist/coremon-sign release -key keys/signing.key -dir "$OUT/release"

subst() { # src dst sha
  sed -e "s|@REPO@|$REPO|g" -e "s|@VERSION@|$VERSION|g" -e "s|@SHA256@|$3|g" "$1" > "$2"
}
cp deploy/coremon-master.service deploy/coremon-master-update.service deploy/coremon-master-update.path "$OUT/"
subst deploy/install-master.sh "$OUT/install-master.sh" ""

tar -C dist -czf "dist/$NAME.tar.gz" "$NAME"
SHA="$(sha256sum "dist/$NAME.tar.gz" | cut -d' ' -f1)"
subst deploy/install-master.sh dist/install-master.sh "$SHA"
(cd dist && sha256sum "$NAME.tar.gz" install-master.sh > SHA256SUMS)

echo
echo "Release ready: dist/$NAME.tar.gz (+ dist/install-master.sh, dist/SHA256SUMS)"
