#!/usr/bin/env bash
# 清理拓扑 + 可选删样本。
# 用法: cleanup.sh <topology> [--results]
#   topology: container | vm | all
#   --results: 同时清除 .results/
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
TOPO="${1:?用法: cleanup.sh <container|vm|all> [--results]}"
CLEAN_RESULTS="${2:-}"

case "$TOPO" in
  container|all)
    echo "[cleanup] 停止容器拓扑..."
    bash "$HERE/stop-container.sh"
    ;;
esac

case "$TOPO" in
  vm|all)
    echo "[cleanup] 停止 VM 拓扑..."
    bash "$HERE/stop-vm.sh"
    ;;
esac

if [[ "$CLEAN_RESULTS" == "--results" ]]; then
  echo "[cleanup] 清除 .results/"
  rm -rf "$ROOT/.results"
fi

echo "[cleanup] done"
