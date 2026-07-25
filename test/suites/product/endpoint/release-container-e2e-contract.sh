#!/usr/bin/env bash
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
RELEASE="$REPO/test/release"

for file in Makefile README.md config.sh doctor.sh run.sh assert.sh attacks/web-runtime-spawns-shell.sh; do
  test -f "$RELEASE/$file"
done

for file in doctor.sh run.sh assert.sh attacks/web-runtime-spawns-shell.sh; do
  test -x "$RELEASE/$file"
done

for image in ubuntu2204 ubuntu2404 debian12; do
  dockerfile="$RELEASE/images/$image/Dockerfile"
  test -f "$dockerfile"
  grep -Fq 'ARG SYSARMOR_INSTALL_URL' "$dockerfile"
  grep -Fq -- '--profile linux-container' "$dockerfile"
  grep -Fq 'ENTRYPOINT ["/usr/local/bin/sysarmor-container-entrypoint"]' "$dockerfile"
done

grep -Fq 'assert.sh' "$RELEASE/run.sh"
grep -Fq 'web-runtime-spawns-shell.sh' "$RELEASE/config.sh"
grep -Fq 'FRESH_DOWNLOAD="${FRESH_DOWNLOAD:-1}"' "$RELEASE/config.sh"
grep -Fq 'RESTART_TEST="${RESTART_TEST:-0}"' "$RELEASE/config.sh"
grep -Fq 'RELEASE_PROXY_URL="${RELEASE_PROXY_URL-https://gh-proxy.org}"' "$RELEASE/config.sh"
grep -Fq -- '--no-cache' "$RELEASE/run.sh"
grep -Fq -- '--privileged' "$RELEASE/run.sh"
grep -Fq -- '--cgroupns=host' "$RELEASE/run.sh"
grep -Fq '/sys/kernel/btf/vmlinux:/sys/kernel/btf/vmlinux:ro' "$RELEASE/run.sh"
grep -Fq '/sys/fs/bpf:/sys/fs/bpf' "$RELEASE/run.sh"
grep -Fq 'run_attack_in_container' "$RELEASE/run.sh"
grep -Fq 'run_attack_in_sibling' "$RELEASE/run.sh"
grep -Fq 'run_attack_on_host' "$RELEASE/run.sh"
grep -Fq 'verify_restart_recovery' "$RELEASE/run.sh"
grep -Fq -- '--connect-timeout "$DOWNLOAD_CONNECT_TIMEOUT"' "$RELEASE/config.sh"
grep -Fq 'sysarmorctl --json event watch' "$RELEASE/assert.sh"
grep -Fq 'sysarmorctl --json signal watch' "$RELEASE/assert.sh"
grep -Fq -- '--include-events' "$RELEASE/assert.sh"
grep -Fq '.missingEventRefs | length' "$RELEASE/assert.sh"
grep -Fq '.eventFrames[]?' "$RELEASE/assert.sh"
grep -Fq '.signalFrame.signal.severity == $severity' "$RELEASE/assert.sh"
grep -Fq 'jq ' "$RELEASE/assert.sh"

if grep -Eq 'sysarmorctl|jq ' "$RELEASE/run.sh"; then
  echo "run.sh must orchestrate without Event/Signal assertions" >&2
  exit 1
fi

for obsolete in common.sh inspect-state.go inspect_state_test.go test-contract.sh; do
  test ! -e "$RELEASE/$obsolete"
done

echo "[release-container-e2e-contract] ok"
