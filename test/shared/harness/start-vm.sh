#!/usr/bin/env bash
# Start a VM environment: vm-endpoint or vm-topology.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
ENV_NAME="${1:-${ENV:-vm-endpoint}}"

case "$ENV_NAME" in
  vm-endpoint|vm-topology) ;;
  *) echo "[start-vm][ERROR] unsupported VM ENV=$ENV_NAME" >&2; exit 2 ;;
esac

echo ">>> 构建 SysArmor binaries"
make -C "$REPO" build

cd "$ROOT/environments/$ENV_NAME"
vagrant up

echo "[start-vm] done ENV=$ENV_NAME"
