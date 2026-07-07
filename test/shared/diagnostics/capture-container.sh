#!/usr/bin/env bash
# 跑容器拓扑场景。默认使用 v2 agent-managed sensor 主路径。
# 前提: docker compose up -d（env/container/）已就绪。
# 用法: capture-container.sh <scenario> [duration_s]
#   scenario: apt-fileless-c2 | apt-staged-drop | benign-ci-noise
#   CAPTURE_MODE=managed | replay
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
RESULTS="$ROOT/.results"
S="${1:?用法: capture-container.sh <scenario> [duration_s]}"
DUR="${2:-30}"
CAPTURE_MODE="${CAPTURE_MODE:-managed}"
: "${C2:=10.66.0.99}" "${GAP:=8}" "${CYCLES:=3}"
mkdir -p "$RESULTS"

echo "[capture-container] 确保容器在跑"
docker start attacker node-a mgr tetragon >/dev/null 2>&1 || true
docker ps --format '{{.Names}}' | tr '\n' ' '; echo
NODE_A_DOCKER="$(docker inspect node-a --format '{{.Id}}' | cut -c1-16)"
echo "[capture-container] 仅保留 node-a docker=$NODE_A_DOCKER 的 Tetragon 事件"

run_attack() {
  if [[ -x "$ROOT/data/scenarios/container/$S/attack.sh" || -f "$ROOT/data/scenarios/container/$S/attack.sh" ]]; then
    C2="$C2" GAP="$GAP" CYCLES="$CYCLES" bash "$ROOT/data/scenarios/container/$S/attack.sh"
  else
    echo "[capture-container] $S 无 attack.sh，执行 replay-only smoke"
  fi
}

if [[ "$CAPTURE_MODE" == "managed" ]]; then
  echo "[capture-container] v2 managed daemon 主路径: $S（窗口 ${DUR}s）"
  WORK="/tmp/sysarmor-capture-$S"
  TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
  TETRA_PATH="$(docker exec tetragon sh -c 'command -v tetra' | tr -d '\r' | tail -1)"
  docker exec tetragon sh -c "tetra tracingpolicy delete sysarmor-syscall-capture 2>/dev/null || true; tetra tracingpolicy delete sysarmor-runtime-collection 2>/dev/null || true"
  docker exec tetragon sh -c "rm -rf '$WORK'; mkdir -p"
  docker exec tetragon sh -c "cat > '$WORK/policy.yaml' <<'EOF'
{"behaviors":["process.exec","network.connect","file.open","file.write","file.chmod"],"observe_only":true}
EOF
cat > '$WORK/agent.yaml' <<EOF
agent:
  id: container-node-a
  host_id: container-node-a
  tenant_id: default
  token: $TOKEN
  label.scenario: $S

manager:
  address: 10.66.0.14:9444
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  tetra_path: $TETRA_PATH
  policy_path: $WORK/policy.yaml
  scope:
    type: container
    selector: $NODE_A_DOCKER
  observe_only: true
  restart: never
  max_restarts: 1
  restart_window: 1h

telemetry:
  batch_size: 256
  flush_interval: 200ms

data_plane:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 500ms
EOF"
  curl -sf -X POST "http://127.0.0.1:19443/api/v1/reset?label=scenario=$S" >/dev/null
  docker exec tetragon sh -c "rm -f '$WORK/agent.log'; /opt/sysarmor/bin/sysarmor-agent run --config '$WORK/agent.yaml' > '$WORK/agent.log' 2>&1 & echo \$! > '$WORK/agent.pid'"
  cleanup_agent() {
    docker exec tetragon sh -c "if [ -f '$WORK/agent.pid' ]; then kill \"\$(cat '$WORK/agent.pid')\" 2>/dev/null || true; fi" >/dev/null 2>&1 || true
  }
  trap cleanup_agent EXIT
  sleep 1
  if ! docker exec tetragon tetra tracingpolicy list | grep -Fq 'sysarmor-runtime-collection'; then
    echo "[capture-container][ERROR] agent-owned runtime TracingPolicy was not applied"
    docker exec tetragon cat "$WORK/agent.log" >&2 2>/dev/null || true
    exit 1
  fi
  docker exec node-a /bin/true >/dev/null 2>&1 || true
  deadline=$((SECONDS + 30))
  until [[ "$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager events list --label scenario="$S" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')" -gt 0 ]]; do
    if (( SECONDS >= deadline )); then
      echo "[capture-container][ERROR] agent-managed Tetra subscription did not become ready"
      docker exec tetragon cat "$WORK/agent.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
  curl -sf -X POST "http://127.0.0.1:19443/api/v1/reset?label=scenario=$S" >/dev/null
  run_attack
  sleep "$DUR"
  cleanup_agent
  docker exec tetragon cat "$WORK/agent.log" > "$RESULTS/$S.container.agent.log" 2>/dev/null || true
  EVENTS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager events list --label scenario="$S" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
  ENDPOINT_SIGNALS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager signals list --label scenario="$S" --layer endpoint --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
  CLOUD_SIGNALS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager signals list --label scenario="$S" --layer cloud --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
  INCIDENTS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager incidents list --label scenario="$S" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("incidents", [])))')"
  python3 - "$RESULTS/container.$S.managed.json" "$S" "$EVENTS" "$ENDPOINT_SIGNALS" "$CLOUD_SIGNALS" "$INCIDENTS" <<'PY'
import json, sys
path, scenario = sys.argv[1], sys.argv[2]
events, endpoint, cloud, incidents = map(int, sys.argv[3:7])
json.dump({"topology":"container","scenario":scenario,"managed_events":events,"managed_endpoint_signals":endpoint,"managed_cloud_signals":cloud,"managed_incidents":incidents,"pass":1 if events > 0 else 0,"fail":0 if events > 0 else 1,"skip":0}, open(path, "w"))
PY
  if [[ "$EVENTS" -le 0 ]]; then
    echo "[capture-container][ERROR] managed daemon uploaded 0 events"
    docker exec tetragon cat "$WORK/agent.log" >&2 2>/dev/null || true
    exit 1
  fi
  echo "[capture-container] managed data_plane: events=$EVENTS endpoint=$ENDPOINT_SIGNALS cloud=$CLOUD_SIGNALS incidents=$INCIDENTS"
  exit 0
fi

if [[ "$CAPTURE_MODE" != "replay" ]]; then
  echo "[capture-container][ERROR] unsupported CAPTURE_MODE=$CAPTURE_MODE"
  exit 1
fi

echo "[capture-container] v1 replay/stream 调试路径: $S（窗口 ${DUR}s）"
POLICY="$ROOT/environments/resources/syscall-capture.yaml"
docker cp "$POLICY" tetragon:/tmp/p.yaml 2>/dev/null || true
docker exec tetragon tetra tracingpolicy add /tmp/p.yaml 2>/dev/null | tail -1 || true
curl -sf -X POST "http://127.0.0.1:19443/api/v1/reset?label=scenario=$S-stream" >/dev/null
docker exec -e NODE_A_DOCKER="$NODE_A_DOCKER" tetragon sh -c "rm -f /tmp/cap-$S.json; timeout $DUR tetra getevents -o json 2>/dev/null | grep -F '\"docker\":\"'\$NODE_A_DOCKER | tee /tmp/cap-$S.json | /opt/sysarmor/bin/sysarmor-agent --manager 10.66.0.14:9444 --agent-id container-node-a-stream --host-id container-node-a --label scenario=$S-stream --stream-jsonl - --batch-size 256 --flush-interval 1s" &
CAP=$!
sleep 3
run_attack
sleep 4
wait $CAP 2>/dev/null || true

docker exec tetragon cat /tmp/cap-$S.json > "$RESULTS/$S.container.tetragon.jsonl"
echo "[capture-container] 落盘: $RESULTS/$S.container.tetragon.jsonl ($(wc -l < "$RESULTS/$S.container.tetragon.jsonl") 行)"

STREAM_EVENTS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager events list --label scenario="$S-stream" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
STREAM_ENDPOINT_SIGNALS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager signals list --label scenario="$S-stream" --layer endpoint --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
STREAM_CLOUD_SIGNALS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager signals list --label scenario="$S-stream" --layer cloud --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')"
STREAM_INCIDENTS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager incidents list --label scenario="$S-stream" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin).get("incidents", [])))')"
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
python3 "$ROOT/shared/fixtures/replay_scenario.py" --scenario "$S" --out "$RESULTS/$S.sensor.jsonl"
docker cp "$RESULTS/$S.sensor.jsonl" mgr:/tmp/$S.sensor.jsonl
curl -sf -X POST "http://127.0.0.1:19443/api/v1/reset?label=scenario=$S" >/dev/null
docker exec mgr /opt/sysarmor/bin/sysarmor-agent \
  --manager 10.66.0.14:9444 \
  --agent-id container-node-a \
  --host-id container-node-a \
  --label scenario="$S" \
  --input-jsonl /tmp/$S.sensor.jsonl
