#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-agent-backpressure)"
sa_pick_ports 31000 4000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SA_TEST_NAME="e2e-agent-backpressure"
SA_WAIT_LOGS=("$TMP/agent.log" "$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref AGENT_PID
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-backpressure] building binaries"
sa_build_all

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-backpressure
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
  max_bytes: 1
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

sa_start_memory_manager --dev-token "$TOKEN"

wait_contains() {
  sa_wait_contains "$@"
}

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/healthz.json"

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

wait_contains "agent-health degraded" '"status":"degraded"' "$RESULTS/e2e-agent-backpressure.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-backpressure --tenant-id default
wait_contains "agent-health backpressure count" '"backpressure_count":1' "$RESULTS/e2e-agent-backpressure.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-backpressure --tenant-id default
wait_contains "agent-health dropped batches" '"dropped_batches":1' "$RESULTS/e2e-agent-backpressure.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-backpressure --tenant-id default
wait_contains "agent-health last error" 'spool max_bytes exceeded' "$RESULTS/e2e-agent-backpressure.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-backpressure --tenant-id default

if compgen -G "$TMP/spool/*.batch.json" >/dev/null; then
  echo "[e2e-agent-backpressure][ERROR] expected no persisted batches after forced drop" >&2
  ls -la "$TMP/spool" >&2
  exit 1
fi

cp "$TMP/agent.log" "$RESULTS/e2e-agent-backpressure.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-backpressure.manager.log"

echo "[e2e-agent-backpressure] ok"
