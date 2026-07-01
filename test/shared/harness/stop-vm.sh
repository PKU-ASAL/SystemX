#!/usr/bin/env bash
# Stop a VM environment: vm-endpoint or vm-topology.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
ENV_NAME="${1:-${ENV:-vm-endpoint}}"

case "$ENV_NAME" in
  vm-endpoint|vm-topology) ;;
  *) echo "[stop-vm][ERROR] unsupported VM ENV=$ENV_NAME" >&2; exit 2 ;;
esac

cd "$ROOT/environments/$ENV_NAME"
vagrant destroy -f
for net in env0 vagrant-libvirt; do
  virsh net-destroy "$net" 2>/dev/null || true
  virsh net-undefine "$net" 2>/dev/null || true
done
echo "[stop-vm] done ENV=$ENV_NAME"
