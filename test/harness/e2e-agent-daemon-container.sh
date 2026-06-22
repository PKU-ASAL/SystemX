#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
RESULTS="$ROOT/.results"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"

mkdir -p "$RESULTS"

cleanup() {
  docker exec mgr sh -c 'if [ -f /tmp/sysarmor-agent-container.pid ]; then kill "$(cat /tmp/sysarmor-agent-container.pid)" 2>/dev/null || true; fi' >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[e2e-agent-daemon-container] starting container topology"
bash "$HERE/start-container.sh" >/dev/null

echo "[e2e-agent-daemon-container] preparing daemon config in mgr container"
docker exec mgr sh -c 'mkdir -p /tmp/sysarmor-agent-container/spool'
docker exec mgr sh -c 'cat > /tmp/sysarmor-agent-container/policy.yaml <<EOF
{"behaviors":["process.exec"],"observe_only":true}
EOF
cat > /tmp/sysarmor-agent-container/agent.yaml <<EOF
agent:
  id: container-agent-daemon
  host_id: container-host
  tenant_id: default
  token: '"$TOKEN"'

manager:
  address: http://127.0.0.1:9443
  transport: grpc

sensor:
  backend: fake
  mode: managed
  policy_path: /tmp/sysarmor-agent-container/policy.yaml
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

spool:
  path: /tmp/sysarmor-agent-container/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 100ms

data_plane:
  retry_initial: 50ms
  retry_max: 100ms
  request_timeout: 2s

health:
  interval: 100ms
EOF'

docker exec mgr curl -sf -X POST "http://127.0.0.1:9443/api/v1/reset" >/dev/null
docker exec mgr sh -c 'rm -f /tmp/sysarmor-agent-container/agent.log; /opt/sysarmor/bin/sysarmor-agent run --config /tmp/sysarmor-agent-container/agent.yaml > /tmp/sysarmor-agent-container/agent.log 2>&1 & echo $! > /tmp/sysarmor-agent-container.pid'

wait_contains() {
  local cmd_name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 20))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-daemon-container][ERROR] timeout waiting for $needle via $cmd_name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- agent log ---" >&2
      docker exec mgr cat /tmp/sysarmor-agent-container/agent.log >&2 2>/dev/null || true
      echo "--- manager health ---" >&2
      docker exec mgr curl -s http://127.0.0.1:9443/healthz >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "agent-health" '"agent_id":"container-agent-daemon"' "$RESULTS/e2e-agent-daemon-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id container-agent-daemon --tenant-id default
wait_contains "metrics" '"events_ingested":1' "$RESULTS/e2e-agent-daemon-container.metrics.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --manager-url 127.0.0.1:9443 --json manager metrics

docker exec mgr cat /tmp/sysarmor-agent-container/agent.log > "$RESULTS/e2e-agent-daemon-container.agent.log"

echo "[e2e-agent-daemon-container] ok"
