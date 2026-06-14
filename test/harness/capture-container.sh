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
NODE_A_DOCKER="$(docker inspect node-a --format '{{.Id}}' | cut -c1-16)"
echo "[capture-container] 仅保留 node-a docker=$NODE_A_DOCKER 的 Tetragon 事件"

# 加载 TracingPolicy（幂等）
POLICY="$ROOT/env/resources/syscall-capture.yaml"
docker cp "$POLICY" tetragon:/tmp/p.yaml 2>/dev/null || true
docker exec tetragon tetra tracingpolicy add /tmp/p.yaml 2>/dev/null | tail -1 || true

echo "[capture-container] 抓事件并运行场景: $S（窗口 ${DUR}s）"
docker exec mgr curl -sf -X POST "http://127.0.0.1:9443/api/v1/reset?scenario=$S-stream" >/dev/null
docker exec -e NODE_A_DOCKER="$NODE_A_DOCKER" tetragon sh -c "rm -f /tmp/cap-$S.json; timeout $DUR tetra getevents -o json 2>/dev/null | grep -F '\"docker\":\"'\$NODE_A_DOCKER | tee /tmp/cap-$S.json | /opt/sysarmor/bin/sysarmor-agent --manager http://10.66.0.10:9443 --agent-id container-node-a-stream --host-id container-node-a --scenario $S-stream --stream-jsonl - --batch-size 256 --flush-interval 1s" &
CAP=$!
sleep 3
if [[ -x "$ROOT/scenarios/container/$S/attack.sh" || -f "$ROOT/scenarios/container/$S/attack.sh" ]]; then
  C2="$C2" GAP="$GAP" CYCLES="$CYCLES" bash "$ROOT/scenarios/container/$S/attack.sh"
else
  echo "[capture-container] $S 无 attack.sh，执行 replay-only smoke"
fi
sleep 4
wait $CAP 2>/dev/null || true

docker exec tetragon cat /tmp/cap-$S.json > "$RESULTS/$S.container.tetragon.jsonl"
echo "[capture-container] 落盘: $RESULTS/$S.container.tetragon.jsonl ($(wc -l < "$RESULTS/$S.container.tetragon.jsonl") 行)"

STREAM_EVENTS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 events --scenario "$S-stream" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
STREAM_ENDPOINT_SIGNALS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 signals --scenario "$S-stream" --layer endpoint --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
STREAM_CLOUD_SIGNALS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 signals --scenario "$S-stream" --layer cloud --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
STREAM_INCIDENTS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 incidents --scenario "$S-stream" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("incidents", [])))')"
python3 - "$RESULTS/container.$S.stream.json" "$S" "$STREAM_EVENTS" "$STREAM_ENDPOINT_SIGNALS" "$STREAM_CLOUD_SIGNALS" "$STREAM_INCIDENTS" <<'PY'
import json, sys
path, scenario = sys.argv[1], sys.argv[2]
events, endpoint, cloud, incidents = map(int, sys.argv[3:7])
json.dump({"topology":"container","scenario":scenario,"stream_events":events,"stream_endpoint_signals":endpoint,"stream_cloud_signals":cloud,"stream_incidents":incidents,"pass":1 if events > 0 else 0,"fail":0 if events > 0 else 1,"skip":0}, open(path, "w"))
PY
if [[ "$STREAM_EVENTS" -le 0 ]]; then
  echo "[capture-container][ERROR] stream smoke uploaded 0 events"
  exit 1
fi

echo "[capture-container] 生成 Phase1 replay events 并上传 manager"
python3 "$ROOT/harness/replay_scenario.py" --scenario "$S" --out "$RESULTS/$S.sensor.jsonl"
docker cp "$RESULTS/$S.sensor.jsonl" mgr:/tmp/$S.sensor.jsonl
docker exec mgr curl -sf -X POST "http://127.0.0.1:9443/api/v1/reset?scenario=$S" >/dev/null
docker exec mgr /opt/sysarmor/bin/sysarmor-agent \
  --manager http://127.0.0.1:9443 \
  --agent-id container-node-a \
  --host-id container-node-a \
  --scenario "$S" \
  --input-jsonl /tmp/$S.sensor.jsonl
