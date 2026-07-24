#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

make_fake_executable() {
  local path="$1"
  mkdir -p "$(dirname "$path")"
  printf '#!/usr/bin/env sh\nexit 0\n' >"$path"
  chmod 0755 "$path"
}

make_fake_executable "$WORK/bin/sysarmor-agent"
make_fake_executable "$WORK/bin/sysarmorctl"
make_fake_executable "$WORK/tetragon/tetragon-v1.7.0/bin/tetragon"
make_fake_executable "$WORK/tetragon/tetragon-v1.7.0/bin/tetra"
tar -C "$WORK/tetragon" -czf "$WORK/tetragon.tar.gz" tetragon-v1.7.0
openssl genrsa -out "$WORK/signing-key.pem" 2048 >/dev/null 2>&1

archive="$WORK/sysarmor-agent-linux-amd64-test.tar.gz"
SYSARMOR_TETRAGON_SHA256="$(sha256sum "$WORK/tetragon.tar.gz" | awk '{print $1}')" \
  "$REPO/deployments/agent/package-agent.sh" \
  --version test \
  --output "$archive" \
  --agent-bin "$WORK/bin/sysarmor-agent" \
  --ctl-bin "$WORK/bin/sysarmorctl" \
  --tetragon-archive "$WORK/tetragon.tar.gz" \
  --signing-key "$WORK/signing-key.pem" >/dev/null

mkdir -p "$WORK/release"
tar -xzf "$archive" -C "$WORK/release"
test -x "$WORK/release/install.sh"
grep -Fq 'Mulan Permissive Software License' "$WORK/release/LICENSE"

install_env=(
  SYSARMOR_ENABLE_SERVICE=0
  SYSARMOR_AGENT_HOME="$WORK/root/opt/sysarmor/agent"
  SYSARMOR_CTL_DST="$WORK/root/usr/local/bin/sysarmorctl"
  SYSARMOR_SERVICE_DST="$WORK/root/etc/systemd/system/sysarmor-agent.service"
  SYSARMOR_CONFIG_DST="$WORK/root/etc/sysarmor/agent/agent.yaml"
  SYSARMOR_POLICY_DST="$WORK/root/etc/sysarmor/agent/policy.json"
  SYSARMOR_STATE_DIR="$WORK/root/var/lib/sysarmor/agent"
  SYSARMOR_RUNTIME_DIR="$WORK/root/run/sysarmor/agent"
  SYSARMOR_TETRAGON_BUNDLE_DIR="$WORK/root/opt/sysarmor/agent/bundles/tetragon"
  SYSARMOR_TETRAGON_INSTALL_DIR="$WORK/root/opt/sysarmor/agent/sensors"
)
env "${install_env[@]}" "$WORK/release/install.sh" >/dev/null

test -x "$WORK/root/opt/sysarmor/agent/bin/sysarmor-agent"
test -x "$WORK/root/usr/local/bin/sysarmorctl"
test -x "$WORK/root/opt/sysarmor/agent/bundles/tetragon/bin/tetragon"
grep -Fq 'state_path: /var/lib/sysarmor/agent' "$WORK/root/etc/sysarmor/agent/agent.yaml"

printf 'preserved-config\n' >"$WORK/root/etc/sysarmor/agent/agent.yaml"
printf 'preserved-policy\n' >"$WORK/root/etc/sysarmor/agent/policy.json"
env "${install_env[@]}" "$WORK/release/install.sh" >/dev/null
grep -Fxq 'preserved-config' "$WORK/root/etc/sysarmor/agent/agent.yaml"
grep -Fxq 'preserved-policy' "$WORK/root/etc/sysarmor/agent/policy.json"

echo "[standalone-release-package] ok"

thin_archive="$WORK/sysarmor-agent-linux-amd64-thin.tar.gz"
"$REPO/deployments/agent/package-agent.sh" \
  --version test-thin \
  --output "$thin_archive" \
  --agent-bin "$WORK/bin/sysarmor-agent" \
  --ctl-bin "$WORK/bin/sysarmorctl" \
  --tetragon-mode download \
  --signing-key "$WORK/signing-key.pem" >/dev/null

mkdir -p "$WORK/thin-release"
tar -xzf "$thin_archive" -C "$WORK/thin-release"
test ! -e "$WORK/thin-release/sensors/tetragon/bin/tetragon"
test -x "$WORK/thin-release/sensors/tetragon/install-bundle.sh"
rm -rf "$WORK/root/opt/sysarmor/agent/bundles/tetragon"
SYSARMOR_TETRAGON_URL="file://$WORK/tetragon.tar.gz" \
SYSARMOR_TETRAGON_SHA256="$(sha256sum "$WORK/tetragon.tar.gz" | awk '{print $1}')" \
SYSARMOR_TETRAGON_ARCHIVE= \
  env "${install_env[@]}" "$WORK/thin-release/install.sh" >/dev/null
test -x "$WORK/root/opt/sysarmor/agent/bundles/tetragon/bin/tetragon"
printf 'keep\n' >"$WORK/root/opt/sysarmor/agent/bundles/tetragon/existing-marker"
if SYSARMOR_TETRAGON_URL="file://$WORK/tetragon.tar.gz" \
  SYSARMOR_TETRAGON_SHA256="$(printf '0%.0s' {1..64})" \
  SYSARMOR_TETRAGON_ARCHIVE= \
  env "${install_env[@]}" "$WORK/thin-release/install.sh" >/dev/null 2>&1; then
  echo "[standalone-release-package][ERROR] invalid Tetragon checksum was accepted" >&2
  exit 1
fi
grep -Fxq keep "$WORK/root/opt/sysarmor/agent/bundles/tetragon/existing-marker"

echo "[standalone-release-package] thin package ok"
