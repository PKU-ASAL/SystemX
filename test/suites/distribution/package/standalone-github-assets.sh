#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

mkdir -p "$WORK/package"
printf '#!/usr/bin/env bash\nset -euo pipefail\nprintf "installed\\n" >"$SYSARMOR_TEST_MARKER"\n' >"$WORK/package/install.sh"
chmod 0755 "$WORK/package/install.sh"

version="v0.1.0-dev.20260724+abcdef12"
package="sysarmor-agent-linux-amd64-$version.tar.gz"
mkdir -p "$WORK/assets"
tar -C "$WORK/package" -czf "$WORK/assets/$package" .

"$REPO/deployments/packages/build-github-assets.sh" \
  --version "$version" \
  --repository PKU-ASAL/sysarmor \
  --package "$WORK/assets/$package" \
  --output-dir "$WORK/assets"

(cd "$WORK/assets" && sha256sum -c SHA256SUMS)
SYSARMOR_RELEASE_BASE_URL="file://$WORK/assets" \
SYSARMOR_TEST_MARKER="$WORK/installed" \
  bash "$WORK/assets/install.sh"
grep -Fxq installed "$WORK/installed"

printf 'tampered\n' >>"$WORK/assets/$package"
if SYSARMOR_RELEASE_BASE_URL="file://$WORK/assets" \
  SYSARMOR_TEST_MARKER="$WORK/should-not-exist" \
  bash "$WORK/assets/install.sh" >/dev/null 2>&1; then
  echo "[standalone-github-assets][ERROR] tampered package was accepted" >&2
  exit 1
fi
test ! -e "$WORK/should-not-exist"

echo "[standalone-github-assets] ok"
