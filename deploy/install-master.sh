#!/bin/sh
# Coremon master installer / upgrader for Debian/Ubuntu (e.g. a dedicated LXC).
#
# From GitHub (latest release):
#   curl -fsSL https://github.com/@REPO@/releases/latest/download/install-master.sh | sh
# From an extracted release tarball:
#   sh install-master.sh
#
# Options:  --force-key   accept a different release signing key (key rotation)
set -eu

REPO="@REPO@"
VERSION="@VERSION@"
TARBALL_SHA256="@SHA256@"
FORCE_KEY=0
for a in "$@"; do
  case "$a" in
    --force-key) FORCE_KEY=1 ;;
    *) echo "coremon: unknown argument $a" >&2; exit 1 ;;
  esac
done

die() { echo "coremon: $*" >&2; exit 1; }
[ "$(id -u)" = "0" ] || die "must run as root"
command -v systemctl >/dev/null 2>&1 || die "systemd is required"

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

# Locate the release files: next to this script, or download from GitHub.
HERE="$(cd "$(dirname "$0")" 2>/dev/null && pwd || true)"
if [ -z "$HERE" ] || [ ! -d "$HERE/release" ]; then
  case "$REPO" in ""|@*) die "run this script from an extracted release directory" ;; esac
  command -v curl >/dev/null 2>&1 || die "curl is required (apt-get install -y curl)"
  TMPD="$(mktemp -d)"
  trap 'rm -rf "$TMPD"' EXIT
  URL="https://github.com/$REPO/releases/download/master-v$VERSION/coremon-master-$VERSION.tar.gz"
  echo "coremon: downloading $URL"
  curl -fsSL -o "$TMPD/coremon.tar.gz" "$URL"
  if [ -n "$TARBALL_SHA256" ] && [ "${TARBALL_SHA256#@}" = "$TARBALL_SHA256" ]; then
    echo "$TARBALL_SHA256  $TMPD/coremon.tar.gz" | sha256sum -c - >/dev/null 2>&1 || die "checksum mismatch - aborting"
  fi
  tar -xzf "$TMPD/coremon.tar.gz" -C "$TMPD"
  HERE="$TMPD/coremon-master-$VERSION"
fi

BIN="$HERE/release/master/$VERSION/linux-$ARCH/coremon-master"
[ -f "$BIN" ] || die "no master binary for linux-$ARCH in this release"
[ -f "$HERE/release/signing.pub" ] || die "release has no signing.pub"

# Release signing key: root-owned; never silently replaced.
mkdir -p /etc/coremon-master
chmod 755 /etc/coremon-master
if [ -f /etc/coremon-master/signing.pub ] && ! cmp -s /etc/coremon-master/signing.pub "$HERE/release/signing.pub"; then
  [ "$FORCE_KEY" = "1" ] || die "release signing key differs from the installed one (/etc/coremon-master/signing.pub). Use --force-key only if you rotated the key on purpose."
fi
install -m 0644 -o root -g root "$HERE/release/signing.pub" /etc/coremon-master/signing.pub
if [ -n "$REPO" ] && [ "${REPO#@}" = "$REPO" ]; then
  echo "COREMON_GITHUB_REPO=$REPO" > /etc/coremon-master/master.env
fi
touch /etc/coremon-master/master.env

DATA=/var/lib/coremon-master
if ! id coremon >/dev/null 2>&1; then
  useradd --system --home-dir "$DATA" --shell /usr/sbin/nologin coremon
fi

if [ -x /usr/local/bin/coremon-master ]; then
  OLD="$(/usr/local/bin/coremon-master version 2>/dev/null || true)"
  echo "coremon: replacing master ${OLD:-?} with $VERSION"
fi
install -m 0755 "$BIN" /usr/local/bin/coremon-master
mkdir -p "$DATA/release" "$DATA/update"
# Agent versions are downloaded by the master itself from the version index.
# Masters before 0.2 also stored separate modules; those are not used anymore.
rm -rf "$DATA/release/modules"
chown -R coremon:coremon "$DATA"
chmod 700 "$DATA"

install -m 0644 "$HERE/coremon-master.service" /etc/systemd/system/coremon-master.service
install -m 0644 "$HERE/coremon-master-update.service" /etc/systemd/system/coremon-master-update.service
install -m 0644 "$HERE/coremon-master-update.path" /etc/systemd/system/coremon-master-update.path
systemctl daemon-reload
systemctl enable coremon-master.service coremon-master-update.path >/dev/null 2>&1
systemctl restart coremon-master-update.path
systemctl restart coremon-master.service
sleep 3

if ! systemctl is-active --quiet coremon-master.service; then
  echo "coremon: service did not start with sandboxing; retrying without it."
  echo "coremon: (enable the 'nesting' feature on this container for full hardening)"
  mkdir -p /etc/systemd/system/coremon-master.service.d
  cat > /etc/systemd/system/coremon-master.service.d/no-sandbox.conf <<'EOF'
[Service]
ProtectSystem=no
ProtectHome=no
PrivateTmp=no
PrivateDevices=no
EOF
  systemctl daemon-reload
  systemctl restart coremon-master.service
  sleep 3
  systemctl is-active --quiet coremon-master.service || { journalctl -u coremon-master -n 30 --no-pager; die "master failed to start"; }
fi

IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
echo
echo "Coremon master $(/usr/local/bin/coremon-master version) is running."
echo "  UI:       https://${IP:-<this-host>}:8443"
if [ -f "$DATA/initial-password.txt" ]; then
  echo "  password: $(cat "$DATA/initial-password.txt")   (initial; change it under Settings)"
fi
[ -s /etc/coremon-master/master.env ] && echo "  releases: version index of https://github.com/$REPO"
echo
echo "The master downloads the agent versions within a minute (see Releases)."
echo "To monitor this container too, open Settings > Systems > Add a system and"
echo "run the generated command here as root."
