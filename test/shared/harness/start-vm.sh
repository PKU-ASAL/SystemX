#!/usr/bin/env bash
# 启动 VM 拓扑: vagrant up
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"

echo ">>> 构建 SysArmor binaries"
make -C "$REPO" build

cd "$ROOT/environments/vm"
vagrant up

echo "[start-vm] done"
