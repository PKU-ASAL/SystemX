#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$HERE/bundle.env"

BUNDLE_DIR="${SYSARMOR_TETRAGON_BUNDLE_DIR:-/opt/sysarmor/agent/bundles/tetragon}"
ARCHIVE="${SYSARMOR_TETRAGON_ARCHIVE:-}"
URL="${SYSARMOR_TETRAGON_URL:-$TETRAGON_URL}"
VERSION="${SYSARMOR_TETRAGON_VERSION:-$TETRAGON_VERSION}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "[install-tetragon-bundle][ERROR] missing required command: $1" >&2
    exit 1
  }
}

need_cmd tar
need_cmd find
need_cmd sha256sum
need_cmd awk

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [[ -n "$ARCHIVE" ]]; then
  cp "$ARCHIVE" "$tmp/tetragon.tar.gz"
else
  need_cmd curl
  curl -fsSL "$URL" -o "$tmp/tetragon.tar.gz"
fi

tar xzf "$tmp/tetragon.tar.gz" -C "$tmp"

tetragon_bin="$(find "$tmp" -type f -name tetragon -perm /111 -print -quit)"
tetra_bin="$(find "$tmp" -type f -name tetra -perm /111 -print -quit)"
if [[ -z "$tetragon_bin" || -z "$tetra_bin" ]]; then
  echo "[install-tetragon-bundle][ERROR] release archive does not contain executable tetragon/tetra" >&2
  find "$tmp" -maxdepth 4 -type f >&2
  exit 1
fi

release_root="$(dirname "$(dirname "$tetragon_bin")")"
if [[ ! -d "$release_root/bin" ]]; then
  release_root="$(dirname "$tetragon_bin")"
fi

rm -rf "$BUNDLE_DIR"
mkdir -p "$BUNDLE_DIR"
cp -a "$release_root"/. "$BUNDLE_DIR"/
mkdir -p "$BUNDLE_DIR/bin"
if [[ ! -x "$BUNDLE_DIR/bin/tetragon" ]]; then
  cp "$tetragon_bin" "$BUNDLE_DIR/bin/tetragon"
fi
if [[ ! -x "$BUNDLE_DIR/bin/tetra" ]]; then
  cp "$tetra_bin" "$BUNDLE_DIR/bin/tetra"
fi
chmod 0755 "$BUNDLE_DIR/bin/tetragon" "$BUNDLE_DIR/bin/tetra"

tetragon_sha="$(sha256sum "$BUNDLE_DIR/bin/tetragon" | awk '{print $1}')"
tetra_sha="$(sha256sum "$BUNDLE_DIR/bin/tetra" | awk '{print $1}')"
cat > "$BUNDLE_DIR/manifest.json" <<JSON
{
  "version": "$VERSION",
  "files": {
    "bin/tetragon": { "sha256": "$tetragon_sha" },
    "bin/tetra": { "sha256": "$tetra_sha" }
  }
}
JSON

echo "[install-tetragon-bundle] installed $VERSION into $BUNDLE_DIR"
