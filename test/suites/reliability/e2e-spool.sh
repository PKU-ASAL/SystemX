#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-agent-spool)"
sa_pick_ports 24000 20000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SA_TEST_NAME="e2e-agent-spool"
SA_WAIT_LOGS=("$TMP/agent.log" "$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref AGENT_PID
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-spool] building binaries"
sa_build_all

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-spool
  host_id: e2e-host
  tenant_id: default
  token: $TOKEN

manager:
  address: 127.0.0.1:$GRPC_PORT
  transport: grpc

control:
  socket_path: $TMP/agent.sock

sensor:
  backend: fake
  mode: managed
  policy_path: $TMP/policy.yaml
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

spool:
  path: $TMP/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 100ms

content:
  path: $TMP/content

data_plane:
  retry_initial: 50ms
  retry_max: 100ms
  request_timeout: 2s

health:
  interval: 100ms
EOF

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

sa_wait_glob "$TMP/spool/*.batch.json" "local spool batch"
cp "$TMP"/spool/*.batch.json "$RESULTS/e2e-agent-spool.before.batch.json"

sa_start_memory_manager --dev-token "$TOKEN"

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/healthz.json"
sa_wait_url_contains "$MGR_URL/api/v1/metrics" '"events_ingested":1' "$RESULTS/e2e-agent-spool.metrics.json"
sa_wait_url_contains "$MGR_URL/api/v1/agent-health?agent_id=e2e-agent-spool&tenant_id=default" '"agent_id":"e2e-agent-spool"' "$RESULTS/e2e-agent-spool.health.json"

sa_wait_no_glob "$TMP/spool/*.batch.json" "spool drain"

if ! grep -Fq 'connect: connection refused' "$TMP/agent.log"; then
  echo "[e2e-agent-spool][ERROR] expected outage evidence in agent log" >&2
  cat "$TMP/agent.log" >&2
  exit 1
fi

sa_wait_url_contains "$MGR_URL/api/v1/agent-health?agent_id=e2e-agent-spool&tenant_id=default" '"queued_batches":0' "$RESULTS/e2e-agent-spool.health.recovered.json"
sa_wait_url_contains "$MGR_URL/api/v1/agent-health?agent_id=e2e-agent-spool&tenant_id=default" '"remaining_batches":0' "$RESULTS/e2e-agent-spool.health.data_plane.json"
sa_wait_url_contains "$MGR_URL/api/v1/events?" '"agent_id":"e2e-agent-spool"' "$RESULTS/e2e-agent-spool.events.json"

cp "$TMP/agent.log" "$RESULTS/e2e-agent-spool.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-spool.manager.log"

echo "[e2e-agent-spool] ok"
