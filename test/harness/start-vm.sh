#!/usr/bin/env bash
# 启动 VM 拓扑: vagrant up
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"

cd "$ROOT/env/vm"
vagrant up
echo "[start-vm] done"
