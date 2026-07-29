#!/usr/bin/env bash
set -euo pipefail

CASE="${1:?usage: capability.sh <backend|btf|bpffs>}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../../.." && pwd)"
source "$ROOT/test/shared/agent/install.sh"

case "$CASE" in
  backend|btf|bpffs) ;;
  *) echo "unsupported capability case: $CASE" >&2; exit 2 ;;
esac

TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-capability-$CASE.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/bundle/bin" "$TMP/install" "$TMP/btf" "$ROOT/test/.results"

make -C "$ROOT" build-agent-binary >/dev/null
printf '%s\n' '#!/bin/sh' 'exit 0' >"$TMP/bundle/bin/tetragon"
printf '%s\n' '#!/bin/sh' 'exit 0' >"$TMP/bundle/bin/tetra"
chmod +x "$TMP/bundle/bin/tetragon" "$TMP/bundle/bin/tetra"
touch "$TMP/btf/vmlinux"

tetragon_sha="$(sha256sum "$TMP/bundle/bin/tetragon" | awk '{print $1}')"
tetra_sha="$(sha256sum "$TMP/bundle/bin/tetra" | awk '{print $1}')"
if [[ "$CASE" == "backend" ]]; then
  tetragon_sha=deadbeef
  expected="checksum mismatch"
elif [[ "$CASE" == "btf" ]]; then
  expected="btf unavailable"
else
  expected="bpffs unavailable"
fi

cat >"$TMP/bundle/manifest.json" <<EOF
{"version":"capability-$CASE","files":{"bin/tetragon":{"sha256":"$tetragon_sha"},"bin/tetra":{"sha256":"$tetra_sha"}}}
EOF
cp "$ROOT/test/fixtures/agent/policies/default.json" "$TMP/policy.json"

sensor=$'  backend: tetragon\n  mode: managed'
sensor+=$'\n  bundle_dir: '"$TMP/bundle"
sensor+=$'\n  install_dir: '"$TMP/install"
sensor+=$'\n  observe_only: true\n  restart: always\n  max_restarts: 1\n  restart_window: 1h'
if [[ "$CASE" == "btf" ]]; then
  sensor+=$'\n  btf_path: '"$TMP/missing-vmlinux"$'\n  require_btf: true'
elif [[ "$CASE" == "bpffs" ]]; then
  sensor+=$'\n  btf_path: '"$TMP/btf/vmlinux"$'\n  bpffs_path: '"$TMP/missing-bpffs"$'\n  require_btf: true\n  require_bpffs: true'
fi
sa_agent_write_config "$TMP/agent.yaml" "$TMP/state" "$TMP/control.sock" "$TMP/policy.json" "$sensor"

set +e
"$ROOT/dist/bin/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1
rc=$?
set -e
if [[ "$rc" -eq 0 ]] || ! grep -Fq "$expected" "$TMP/agent.log"; then
  cat "$TMP/agent.log" >&2
  echo "capability case $CASE did not fail with $expected" >&2
  exit 1
fi
cp "$TMP/agent.log" "$ROOT/test/.results/capability-$CASE.agent.log"
echo "capability case $CASE passed"
