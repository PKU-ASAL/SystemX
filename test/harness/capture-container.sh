#!/usr/bin/env bash
# 跑容器拓扑场景 + tetragon 抓内核事件 → 落盘 test/.results/<scenario>.tetragon.jsonl。
# 前提: docker compose up -d（env/container/）已就绪。
# 用法: capture-container.sh <scenario> [duration_s]
#   scenario: apt-fileless-c2 | apt-staged-drop | benign-ci-noise
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
RESULTS="$ROOT/.results"
S="${1:?用法: capture-container.sh <scenario> [duration_s]}"
DUR="${2:-30}"
: "${C2:=10.66.0.99}" "${GAP:=8}" "${CYCLES:=3}"
mkdir -p "$RESULTS"

echo "[capture-container] 确保容器在跑"
docker start attacker node-a mgr tetragon >/dev/null 2>&1 || true
docker ps --format '{{.Names}}' | tr '\n' ' '; echo

# 加载 TracingPolicy（幂等）
POLICY="$ROOT/env/resources/syscall-capture.yaml"
docker cp "$POLICY" tetragon:/tmp/p.yaml 2>/dev/null || true
docker exec tetragon tetra tracingpolicy add /tmp/p.yaml 2>/dev/null | tail -1 || true

echo "[capture-container] 抓事件并运行场景: $S（窗口 ${DUR}s）"
timeout "$DUR" docker exec tetragon tetra getevents -o json > /tmp/cap-container-$S.json 2>/dev/null &
CAP=$!
sleep 3
C2="$C2" GAP="$GAP" CYCLES="$CYCLES" bash "$ROOT/scenarios/container/$S/attack.sh"
sleep 4
kill $CAP 2>/dev/null; wait $CAP 2>/dev/null || true

cp /tmp/cap-container-$S.json "$RESULTS/$S.container.tetragon.jsonl"
echo "[capture-container] 落盘: $RESULTS/$S.container.tetragon.jsonl ($(wc -l < "$RESULTS/$S.container.tetragon.jsonl") 行)"
