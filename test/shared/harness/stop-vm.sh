#!/usr/bin/env bash
# Stop a VM environment: vm-endpoint or vm-topology.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
ENV_NAME="${1:-${ENV:-vm-endpoint}}"
BEST_EFFORT="${SYSARMOR_VM_STOP_BEST_EFFORT:-0}"

case "$ENV_NAME" in
  vm-endpoint|vm-topology) ;;
  *) echo "[stop-vm][ERROR] unsupported VM ENV=$ENV_NAME" >&2; exit 2 ;;
esac

cd "$ROOT/environments/$ENV_NAME"
if [[ "$BEST_EFFORT" == "1" ]]; then
  tmp_log="$(mktemp)"
  if ! vagrant destroy -f >"$tmp_log" 2>&1; then
    echo "[stop-vm][WARN] vagrant destroy failed for ENV=$ENV_NAME; continuing in best-effort cleanup" >&2
    tail -5 "$tmp_log" >&2 || true
  else
    cat "$tmp_log"
  fi
  rm -f "$tmp_log"
else
  if ! vagrant destroy -f; then
    exit 1
  fi
fi
for net in env0 vagrant-libvirt; do
  virsh net-destroy "$net" 2>/dev/null || true
  virsh net-undefine "$net" 2>/dev/null || true
done
echo "[stop-vm] done ENV=$ENV_NAME"
