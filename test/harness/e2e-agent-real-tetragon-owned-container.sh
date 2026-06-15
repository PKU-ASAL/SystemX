#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
RESULTS="$ROOT/.results"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
WORK="/tmp/sysarmor-agent-owned-container"
SCENARIO="${SCENARIO:-apt-staged-drop-owned-container}"
DUR="${DUR:-10}"
C2="${C2:-10.66.0.99}"
GAP="${GAP:-3}"
OWNED_CONTAINER="tetragon-owned"

mkdir -p "$RESULTS"

cleanup() {
  docker exec "$OWNED_CONTAINER" sh -c 'if [ -f /tmp/sysarmor-agent-owned-container/agent.pid ]; then kill "$(cat /tmp/sysarmor-agent-owned-container/agent.pid)" 2>/dev/null || true; fi' >/dev/null 2>&1 || true
  docker rm -f "$OWNED_CONTAINER" >/dev/null 2>&1 || true
  docker start tetragon >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[e2e-agent-real-tetragon-owned-container] starting container topology"
bash "$HERE/start-container.sh" >/dev/null

NODE_A_DOCKER="$(docker inspect node-a --format '{{.Id}}' | cut -c1-16)"
NETWORK="$(docker inspect mgr --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}')"
echo "[e2e-agent-real-tetragon-owned-container] node-a docker prefix=$NODE_A_DOCKER network=$NETWORK"

echo "[e2e-agent-real-tetragon-owned-container] stopping compose tetragon container"
docker stop tetragon >/dev/null
docker rm -f "$OWNED_CONTAINER" >/dev/null 2>&1 || true

echo "[e2e-agent-real-tetragon-owned-container] starting owned privileged container"
docker run -d \
  --name "$OWNED_CONTAINER" \
  --entrypoint sh \
  --privileged \
  --pid=host \
  --cgroupns=host \
  --network "$NETWORK" \
  -v /sys/kernel/btf/vmlinux:/sys/kernel/btf/vmlinux:ro \
  -v /sys/fs/bpf:/sys/fs/bpf \
  -v "$REPO/bin:/opt/sysarmor/bin:ro" \
  quay.io/cilium/tetragon:v1.7.0 \
  -c 'tail -f /dev/null' >/dev/null

echo "[e2e-agent-real-tetragon-owned-container] preparing agent daemon config in owned container"
docker exec "$OWNED_CONTAINER" sh -c "rm -rf '$WORK'; mkdir -p '$WORK/spool'"
TETRA_PATH="$(docker exec "$OWNED_CONTAINER" sh -c 'command -v tetra' | tr -d '\r' | tail -1)"
TETRAGON_PATH="$(docker exec "$OWNED_CONTAINER" sh -c 'command -v tetragon' | tr -d '\r' | tail -1)"
if [[ -z "$TETRA_PATH" || -z "$TETRAGON_PATH" ]]; then
  echo "[e2e-agent-real-tetragon-owned-container][ERROR] tetra/tetragon not found in owned container" >&2
  exit 1
fi
docker exec "$OWNED_CONTAINER" sh -c "cat > '$WORK/policy.yaml' <<'EOF'
kinds: [EXEC, CONNECT, OPEN, WRITE, CHMOD]
EOF
cat > '$WORK/agent.yaml' <<EOF
agent:
  id: container-node-a-owned
  host_id: container-node-a
  tenant_id: default
  token: $TOKEN
  scenario: $SCENARIO

manager:
  address: http://10.66.0.10:9443
  transport: http

sensor:
  backend: tetragon
  mode: managed
  tetra_path: $TETRA_PATH
  tetragon_path: $TETRAGON_PATH
  policy_path: $WORK/policy.yaml
  scope:
    type: container
    selector: $NODE_A_DOCKER
  observe_only: true
  restart: always
  max_restarts: 3
  restart_window: 500ms

spool:
  path: $WORK/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 200ms

upload:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 500ms
EOF"

docker exec mgr curl -sf -X POST "http://127.0.0.1:9443/api/v1/reset?scenario=$SCENARIO" >/dev/null
docker exec "$OWNED_CONTAINER" sh -c "rm -f '$WORK/agent.log'; /opt/sysarmor/bin/sysarmor-agent run --config '$WORK/agent.yaml' > '$WORK/agent.log' 2>&1 & echo \$! > '$WORK/agent.pid'"

wait_contains() {
  local cmd_name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 60))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-real-tetragon-owned-container][ERROR] timeout waiting for $needle via $cmd_name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- agent log ---" >&2
      docker exec "$OWNED_CONTAINER" cat "$WORK/agent.log" >&2 2>/dev/null || true
      echo "--- tetragon process ---" >&2
      docker exec "$OWNED_CONTAINER" sh -c "ps -ef | grep tetragon | grep -v grep || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

wait_process_absent() {
  local label="$1"
  local pattern="$2"
  local out="$3"
  local deadline=$((SECONDS + 30))
  while docker exec -e SYSARMOR_PROCESS_PATTERN="$pattern" "$OWNED_CONTAINER" sh -c 'ps -ef | grep -F "$SYSARMOR_PROCESS_PATTERN" | grep -v grep' >"$out" 2>"$out.err"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-real-tetragon-owned-container][ERROR] $label process still present after agent stop" >&2
      echo "--- matching process ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- agent log ---" >&2
      docker exec "$OWNED_CONTAINER" cat "$WORK/agent.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
  : >"$out"
}

wait_contains "agent-health backend" '"backend":"tetragon"' "$RESULTS/e2e-agent-real-tetragon-owned-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-health capability kernel" '"kernel_release":' "$RESULTS/e2e-agent-real-tetragon-owned-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-health capability btf" '"btf_available":true' "$RESULTS/e2e-agent-real-tetragon-owned-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-health capability bpffs" '"bpffs_available":true' "$RESULTS/e2e-agent-real-tetragon-owned-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-health policy" '"policy_loaded":true' "$RESULTS/e2e-agent-real-tetragon-owned-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-owned tracing policy" 'sysarmor-runtime-collection' "$RESULTS/e2e-agent-real-tetragon-owned-container.tracingpolicy.txt" \
  docker exec "$OWNED_CONTAINER" tetra tracingpolicy list
wait_contains "agent-owned tetragon process" "$TETRAGON_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-container.ps.txt" \
  docker exec "$OWNED_CONTAINER" sh -c "ps -ef | grep tetragon | grep -v grep"
deadline=$((SECONDS + 30))
until [[ "$(docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 events --scenario "$SCENARIO" --json | python3 -c 'import json,sys; print(len(json.load(sys.stdin)))')" -gt 0 ]]; do
  if (( SECONDS >= deadline )); then
    echo "[e2e-agent-real-tetragon-owned-container][ERROR] owned Tetra subscription did not become ready" >&2
    docker exec "$OWNED_CONTAINER" cat "$WORK/agent.log" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
done
docker exec mgr curl -sf -X POST "http://127.0.0.1:9443/api/v1/reset?scenario=$SCENARIO" >/dev/null

echo "[e2e-agent-real-tetragon-owned-container] running apt-staged-drop attack"
C2="$C2" GAP="$GAP" bash "$ROOT/scenarios/container/apt-staged-drop/attack.sh"
sleep "$DUR"

wait_contains "scenario events" "\"scenario\":\"$SCENARIO\"" "$RESULTS/e2e-agent-real-tetragon-owned-container.events.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json events --scenario "$SCENARIO"
wait_contains "cloud cross-lineage signal" 'dropped_payload_executed_and_connects' "$RESULTS/e2e-agent-real-tetragon-owned-container.cloud-signals.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json signals --scenario "$SCENARIO" --layer cloud
wait_contains "incident" '"incidents":[{' "$RESULTS/e2e-agent-real-tetragon-owned-container.incidents.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json incidents --scenario "$SCENARIO"

PID_BEFORE="$(docker exec "$OWNED_CONTAINER" sh -c "cat '$WORK/agent.pid'" 2>/dev/null | tr -d '\r' | tail -1)"
if [[ -z "$PID_BEFORE" ]]; then
  echo "[e2e-agent-real-tetragon-owned-container][ERROR] agent pid file is empty before restart" >&2
  exit 1
fi

echo "[e2e-agent-real-tetragon-owned-container] verifying manual restart keeps owned real Tetragon path healthy"
docker exec "$OWNED_CONTAINER" sh -c "kill -TERM $PID_BEFORE" >/dev/null
wait_contains "agent-health degraded after stop" '"status":"degraded"' "$RESULTS/e2e-agent-real-tetragon-owned-container.health-degraded.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-health queued batches after stop" '"queued_batches":0' "$RESULTS/e2e-agent-real-tetragon-owned-container.health-after-stop.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-health remaining batches after stop" '"remaining_batches":0' "$RESULTS/e2e-agent-real-tetragon-owned-container.health-after-stop.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
deadline=$((SECONDS + 30))
until ! docker exec "$OWNED_CONTAINER" sh -c "kill -0 $PID_BEFORE" >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "[e2e-agent-real-tetragon-owned-container][ERROR] agent did not stop after TERM" >&2
    docker exec "$OWNED_CONTAINER" cat "$WORK/agent.log" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
done
wait_process_absent "owned tetragon" "$TETRAGON_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-container.ps-after-stop.txt"
wait_process_absent "owned tetra getevents" "$TETRA_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-container.tetra-after-stop.txt"

docker exec "$OWNED_CONTAINER" sh -c "rm -f '$WORK/agent.log'; /opt/sysarmor/bin/sysarmor-agent run --config '$WORK/agent.yaml' >> '$WORK/agent.log' 2>&1 & echo \$! > '$WORK/agent.pid'"

wait_contains "agent-health recovered after restart" '"status":"ok"' "$RESULTS/e2e-agent-real-tetragon-owned-container.health-after-restart.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-health capability after restart" '"kernel_release":' "$RESULTS/e2e-agent-real-tetragon-owned-container.health-after-restart.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-health running after restart" '"running":true' "$RESULTS/e2e-agent-real-tetragon-owned-container.health-after-restart.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "agent-health policy after restart" '"policy_loaded":true' "$RESULTS/e2e-agent-real-tetragon-owned-container.health-after-restart.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-node-a-owned --tenant-id default
wait_contains "owned tetragon process after restart" "$TETRAGON_PATH" "$RESULTS/e2e-agent-real-tetragon-owned-container.ps-after-restart.txt" \
  docker exec "$OWNED_CONTAINER" sh -c "ps -ef | grep tetragon | grep -v grep"

docker exec "$OWNED_CONTAINER" cat "$WORK/agent.log" > "$RESULTS/e2e-agent-real-tetragon-owned-container.agent.log"

echo "[e2e-agent-real-tetragon-owned-container] ok"
