#!/usr/bin/env bash
# 停止容器拓扑: compose down -v
set -euo pipefail
cd "$(dirname "$0")"/../env/container
docker compose down -v
echo "[stop-container] done"
