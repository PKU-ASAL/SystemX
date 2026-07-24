#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERSION=""
REPOSITORY=""
PACKAGE=""
OUTPUT_DIR=""

usage() {
  echo "usage: build-github-assets.sh --version VERSION --repository OWNER/REPO --package FILE --output-dir DIR"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --repository) REPOSITORY="$2"; shift 2 ;;
    --package) PACKAGE="$2"; shift 2 ;;
    --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "[build-github-assets][ERROR] unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

[[ "$VERSION" =~ ^[A-Za-z0-9._+-]+$ ]] || { echo "[build-github-assets][ERROR] invalid version" >&2; exit 2; }
[[ "$REPOSITORY" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]] || { echo "[build-github-assets][ERROR] invalid repository" >&2; exit 2; }
[[ -f "$PACKAGE" ]] || { echo "[build-github-assets][ERROR] package not found: $PACKAGE" >&2; exit 1; }
[[ -n "$OUTPUT_DIR" ]] || { echo "[build-github-assets][ERROR] output directory is required" >&2; exit 2; }

mkdir -p "$OUTPUT_DIR"
package_name="$(basename "$PACKAGE")"
output_package="$OUTPUT_DIR/$package_name"
if [[ "$(realpath "$PACKAGE")" != "$(realpath -m "$output_package")" ]]; then
  install -m 0644 "$PACKAGE" "$output_package"
fi
sha256="$(sha256sum "$output_package" | awk '{print $1}')"
printf '%s  %s\n' "$sha256" "$package_name" >"$OUTPUT_DIR/SHA256SUMS"

sed \
  -e "s|@VERSION@|$VERSION|g" \
  -e "s|@REPOSITORY@|$REPOSITORY|g" \
  -e "s|@PACKAGE@|$package_name|g" \
  -e "s|@SHA256@|$sha256|g" \
  "$HERE/install-github.sh.in" >"$OUTPUT_DIR/install.sh"
chmod 0755 "$OUTPUT_DIR/install.sh"

echo "$OUTPUT_DIR"
