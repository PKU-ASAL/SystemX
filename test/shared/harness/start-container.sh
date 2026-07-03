#!/usr/bin/env bash
# 启动容器拓扑: compose up
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"

echo ">>> 构建 SysArmor binaries"
make -C "$REPO" build

cd "$ROOT/environments/container"
docker compose up -d --build --force-recreate postgres kafka redis opensearch mgr gateway worker
docker compose up -d attacker node-a tetragon

echo ">>> 等待 manager health"
for i in $(seq 1 20); do
  if curl -sf http://127.0.0.1:19443/healthz >/dev/null; then
    break
  fi
  sleep 1
done

echo ">>> 等待 gateway health"
for i in $(seq 1 20); do
  if curl -sf http://127.0.0.1:19445/healthz >/dev/null; then
    break
  fi
  sleep 1
done

echo "[start-container] done"
