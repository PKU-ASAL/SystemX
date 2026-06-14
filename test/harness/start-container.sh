#!/usr/bin/env bash
# 启动容器拓扑: compose up + 加载 TracingPolicy
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"

echo ">>> 构建 SysArmor binaries"
make -C "$REPO" build

cd "$ROOT/env/container"
docker compose up -d --build --force-recreate mgr
docker compose up -d attacker node-a tetragon

echo ">>> 等待 manager health"
for i in $(seq 1 20); do
  if docker exec mgr curl -sf http://127.0.0.1:9443/healthz >/dev/null; then
    break
  fi
  sleep 1
done

echo ">>> 加载 TracingPolicy"
docker cp "$ROOT/env/resources/syscall-capture.yaml" tetragon:/tmp/p.yaml
docker exec tetragon tetra tracingpolicy add /tmp/p.yaml 2>/dev/null || true
echo "[start-container] done"
