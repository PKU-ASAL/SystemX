#!/usr/bin/env bash
# Clean one test environment and optionally generated results.
# Usage: cleanup.sh <container|vm-endpoint|vm-topology|all> [--results]
#   --results: 同时清除 .results/
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
ENV_NAME="${1:?usage: cleanup.sh <container|vm-endpoint|vm-topology|all> [--results]}"
CLEAN_RESULTS="${2:-}"

case "$ENV_NAME" in
  container|all)
    echo "[cleanup] 停止容器拓扑..."
    bash "$HERE/stop-container.sh"
    ;;
esac

case "$ENV_NAME" in
  vm-endpoint|vm-topology)
    echo "[cleanup] 停止 VM 环境: $ENV_NAME..."
    bash "$HERE/stop-vm.sh" "$ENV_NAME"
    ;;
  all)
    echo "[cleanup] 停止 VM 环境..."
    bash "$HERE/stop-vm.sh" vm-endpoint
    bash "$HERE/stop-vm.sh" vm-topology
    ;;
esac

if [[ "$CLEAN_RESULTS" == "--results" ]]; then
  echo "[cleanup] 清除 .results/"
  rm -rf "$ROOT/.results"
fi

echo "[cleanup] done"
