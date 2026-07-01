#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
RESULTS="$ROOT/.results"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
WORK="/tmp/sysarmor-agent-apt-container"
SCENARIO="${SCENARIO:-apt-fileless-c2-managed}"
DUR="${DUR:-18}"
C2="${C2:-10.66.0.99}"

mkdir -p "$RESULTS"

cleanup() {
  docker exec tetragon sh -c 'if [ -f /tmp/sysarmor-agent-apt-container/agent.pid ]; then kill "$(cat /tmp/sysarmor-agent-apt-container/agent.pid)" 2>/dev/null || true; fi' >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[e2e-agent-apt-container] starting container topology"
bash "$HERE/start-container.sh" >/dev/null

NODE_A_DOCKER="$(docker inspect node-a --format '{{.Id}}' | cut -c1-16)"
echo "[e2e-agent-apt-container] node-a docker prefix=$NODE_A_DOCKER"

echo "[e2e-agent-apt-container] preparing agent daemon config in tetragon container"
docker exec tetragon sh -c "rm -rf '$WORK'; mkdir -p"
TETRA_PATH="$(docker exec tetragon sh -c 'command -v tetra' | tr -d '\r' | tail -1)"
docker exec tetragon sh -c "cat > '$WORK/policy.yaml' <<'EOF'
{"behaviors":["process.exec","network.connect","file.open","file.write","file.chmod"],"observe_only":true}
EOF
cat > '$WORK/agent.yaml' <<EOF
agent:
  id: container-node-a-managed
  host_id: container-node-a
  tenant_id: default
  token: $TOKEN
  label.scenario: $SCENARIO

manager:
  address: http://10.66.0.10:9443
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

docker exec mgr curl -sf -X POST "http://127.0.0.1:9443/api/v1/reset?label=scenario=$SCENARIO" >/dev/null
docker exec tetragon sh -c "rm -f '$WORK/agent.log'; /opt/sysarmor/bin/sysarmor-agent run --config '$WORK/agent.yaml' > '$WORK/agent.log' 2>&1 & echo \$! > '$WORK/agent.pid'"

wait_contains() {
  local cmd_name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 30))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-apt-container][ERROR] timeout waiting for $needle via $cmd_name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- agent log ---" >&2
      docker exec tetragon cat "$WORK/agent.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "agent-health" '"backend":"tetragon"' "$RESULTS/e2e-agent-apt-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id container-node-a-managed --tenant-id default
wait_contains "agent-health installed" '"installed":true' "$RESULTS/e2e-agent-apt-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id container-node-a-managed --tenant-id default
deadline=$((SECONDS + 30))
until [[ "$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager events list --label scenario="$SCENARIO" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')" -gt 0 ]]; do
  if (( SECONDS >= deadline )); then
    echo "[e2e-agent-apt-container][ERROR] managed Tetra subscription did not become ready" >&2
    docker exec tetragon cat "$WORK/agent.log" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
done
docker exec mgr curl -sf -X POST "http://127.0.0.1:9443/api/v1/reset?label=scenario=$SCENARIO" >/dev/null

echo "[e2e-agent-apt-container] running apt-fileless-c2 attack"
C2="$C2" bash "$ROOT/data/scenarios/container/apt-fileless-c2/attack.sh"
sleep "$DUR"

wait_contains "scenario events" "\"labels\":{\"scenario\":\"$SCENARIO\"" "$RESULTS/e2e-agent-apt-container.events.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager events list --label scenario="$SCENARIO"
wait_contains "endpoint reverse shell signal" 'reverse_shell_pattern' "$RESULTS/e2e-agent-apt-container.signals.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager signals list --label scenario="$SCENARIO" --layer endpoint
wait_contains "endpoint payload signal" 'payload_dropped' "$RESULTS/e2e-agent-apt-container.signals.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager signals list --label scenario="$SCENARIO" --layer endpoint
wait_contains "cloud dropped/connect signal" 'dropped_payload_executed_and_connects' "$RESULTS/e2e-agent-apt-container.cloud-signals.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager signals list --label scenario="$SCENARIO" --layer cloud
wait_contains "cloud web shell signal" 'web_shell_chain' "$RESULTS/e2e-agent-apt-container.cloud-signals.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager signals list --label scenario="$SCENARIO" --layer cloud
wait_contains "incident" '"incidents":[{' "$RESULTS/e2e-agent-apt-container.incidents.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager incidents list --label scenario="$SCENARIO"
wait_contains "incident converge" '"method":"rarity+causal-topk"' "$RESULTS/e2e-agent-apt-container.incidents.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager incidents list --label scenario="$SCENARIO"

docker exec tetragon cat "$WORK/agent.log" > "$RESULTS/e2e-agent-apt-container.agent.log"

echo "[e2e-agent-apt-container] ok"
