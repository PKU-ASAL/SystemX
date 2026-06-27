#!/usr/bin/env bash
# 停止容器拓扑: compose down -v
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
cd "$ROOT/environments/container"
docker compose down -v
echo "[stop-container] done"
