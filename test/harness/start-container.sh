#!/usr/bin/env bash
# 启动容器拓扑: compose up + 加载 TracingPolicy
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"

cd "$ROOT/env/container"
docker compose up -d

echo ">>> 加载 TracingPolicy"
docker cp "$ROOT/env/resources/syscall-capture.yaml" tetragon:/tmp/p.yaml
docker exec tetragon tetra tracingpolicy add /tmp/p.yaml 2>/dev/null || true
echo "[start-container] done"
