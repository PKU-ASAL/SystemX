#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
RESULTS="$ROOT/.results"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
WORK="/tmp/sysarmor-agent-staged-container"
SCENARIO="${SCENARIO:-apt-staged-drop-managed}"
DUR="${DUR:-10}"
C2="${C2:-10.66.0.99}"
POLICY_JSON='{"behaviors":["process.exec","network.connect","file.open","file.write","file.chmod"],"file_prefixes":["/dev/shm","/tmp","/var/tmp","/var/lib/app/plugins"],"observe_only":true}'
GAP="${GAP:-3}"

mkdir -p "$RESULTS"

cleanup() {
  docker exec tetragon sh -c 'if [ -f /tmp/sysarmor-agent-staged-container/agent.pid ]; then kill "$(cat /tmp/sysarmor-agent-staged-container/agent.pid)" 2>/dev/null || true; fi' >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[e2e-agent-staged-container] starting container topology"
if [[ "${SYSARMOR_SKIP_START_CONTAINER:-0}" != "1" ]]; then
  bash "$ROOT/shared/harness/start-container.sh" >/dev/null
fi

NODE_A_DOCKER="$(docker inspect node-a --format '{{.Id}}' | cut -c1-16)"
echo "[e2e-agent-staged-container] node-a docker prefix=$NODE_A_DOCKER"

echo "[e2e-agent-staged-container] preparing agent daemon config in tetragon container"
docker exec tetragon sh -c "if [ -f '$WORK/agent.pid' ]; then kill \"\$(cat '$WORK/agent.pid')\" 2>/dev/null || true; fi; pkill -x sysarmor-agent 2>/dev/null || true; rm -rf '$WORK'; mkdir -p '$WORK'"
TETRA_PATH="$(docker exec tetragon sh -c 'command -v tetra' | tr -d '\r' | tail -1)"
docker exec tetragon sh -c "printf '%s\n' '$POLICY_JSON' > '$WORK/policy.yaml'
cat > '$WORK/agent.yaml' <<EOF
agent:
  id: container-node-a-staged
  host_id: container-node-a
  tenant_id: default
  token: $TOKEN
  label.scenario: $SCENARIO

manager:
  address: 10.66.0.14:9444
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  event_transport: tetra
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
  batch_size: 1
  flush_interval: 200ms

data_plane:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 500ms
EOF"

docker exec node-a curl -sf -X POST "http://mgr:9443/api/v1/reset?label=scenario=$SCENARIO" >/dev/null
docker exec tetragon tetra tracingpolicy delete sysarmor-runtime-collection >/dev/null 2>&1 || true
docker exec tetragon sh -c "rm -f '$WORK/agent.log'; /opt/sysarmor/bin/sysarmor-agent run --config '$WORK/agent.yaml' > '$WORK/agent.log' 2>&1 & echo \$! > '$WORK/agent.pid'"

wait_contains() {
  local cmd_name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 90))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-staged-container][ERROR] timeout waiting for $needle via $cmd_name" >&2
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

wait_contains "agent-health" '"backend":"tetragon"' "$RESULTS/e2e-agent-staged-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id container-node-a-staged --tenant-id default
wait_contains "runtime tracing policy" 'enabled' "$RESULTS/e2e-agent-staged-container.tracingpolicy.txt" \
  docker exec tetragon tetra tracingpolicy list
deadline=$((SECONDS + 90))
until [[ "$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 manager events list --label scenario="$SCENARIO" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')" -gt 0 ]]; do
  if (( SECONDS >= deadline )); then
    echo "[e2e-agent-staged-container][ERROR] managed Tetra subscription did not become ready" >&2
    docker exec tetragon cat "$WORK/agent.log" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
done

echo "[e2e-agent-staged-container] running apt-staged-drop attack"
C2="$C2" GAP="$GAP" bash "$ROOT/data/scenarios/container/apt-staged-drop/attack.sh"
sleep "$DUR"

wait_contains "scenario events" "\"scenario\":\"$SCENARIO\"" "$RESULTS/e2e-agent-staged-container.events.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager events list --label scenario="$SCENARIO"
wait_contains "endpoint payload signal" 'payload_dropped' "$RESULTS/e2e-agent-staged-container.signals.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager signals list --label scenario="$SCENARIO" --layer endpoint
wait_contains "endpoint suspicious connect signal" 'suspicious_exec_connect' "$RESULTS/e2e-agent-staged-container.signals.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager signals list --label scenario="$SCENARIO" --layer endpoint
wait_contains "cloud cross-lineage signal" 'dropped_payload_executed_and_connects' "$RESULTS/e2e-agent-staged-container.cloud-signals.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager signals list --label scenario="$SCENARIO" --layer cloud
wait_contains "cloud cross-lineage marker" '"crossLineage":true' "$RESULTS/e2e-agent-staged-container.cloud-signals.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager signals list --label scenario="$SCENARIO" --layer cloud
wait_contains "incident" '"id":"inc-' "$RESULTS/e2e-agent-staged-container.incidents.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager incidents list --label scenario="$SCENARIO"
wait_contains "incident converge" '"method":"rarity+causal-topk"' "$RESULTS/e2e-agent-staged-container.incidents.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager incidents list --label scenario="$SCENARIO"

TERMINALS="$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager signals list --label scenario="$SCENARIO" --terminal true)"
printf '%s\n' "$TERMINALS" > "$RESULTS/e2e-agent-staged-container.terminals.json"
if [[ "$TERMINALS" != "[]" ]]; then
  echo "[e2e-agent-staged-container][ERROR] expected no endpoint terminal signals" >&2
  printf '%s\n' "$TERMINALS" >&2
  docker exec tetragon cat "$WORK/agent.log" >&2 2>/dev/null || true
  exit 1
fi

docker exec tetragon cat "$WORK/agent.log" > "$RESULTS/e2e-agent-staged-container.agent.log"

echo "[e2e-agent-staged-container] ok"
