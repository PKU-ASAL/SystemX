#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

for file in Makefile doctor.sh run.sh assert-state.sh inspect-state.go README.md; do
  test -f "$HERE/$file"
done
for file in doctor.sh run.sh assert-state.sh; do
  test -x "$HERE/$file"
done

grep -Fq 'doctor:' "$HERE/Makefile"
grep -Fq 'test-contract:' "$HERE/Makefile"
grep -Fq 'test:' "$HERE/Makefile"

for image in ubuntu2204 ubuntu2404 debian12; do
  dockerfile="$HERE/images/$image/Dockerfile"
  test -f "$dockerfile"
  grep -Fq 'ARG SYSARMOR_INSTALL_URL' "$dockerfile"
  grep -Fq -- '--profile linux-container' "$dockerfile"
  grep -Fq 'util-linux' "$dockerfile"
  grep -Fq 'ENTRYPOINT ["/usr/local/bin/sysarmor-container-entrypoint"]' "$dockerfile"
done

grep -Fq -- '--privileged' "$HERE/run.sh"
grep -Fq -- '--cgroupns=host' "$HERE/run.sh"
grep -Fq '/sys/kernel/btf/vmlinux:/sys/kernel/btf/vmlinux:ro' "$HERE/run.sh"
grep -Fq '/sys/fs/bpf:/sys/fs/bpf' "$HERE/run.sh"
grep -Fq 'docker start' "$HERE/run.sh"
grep -Fq 'go build' "$HERE/run.sh"
grep -Fq 'assert-state.sh" positive' "$HERE/run.sh"
grep -Fq 'assert-state.sh" absent' "$HERE/run.sh"

grep -Fq 'web_runtime_spawns_shell' "$HERE/assert-state.sh"
grep -Fq 'docker cp' "$HERE/assert-state.sh"
grep -Fq 'SYSARMOR_STATE_INSPECTOR' "$HERE/assert-state.sh"
grep -Fq 'localstore.Open' "$HERE/inspect-state.go"

echo "[release-contract] ok"
