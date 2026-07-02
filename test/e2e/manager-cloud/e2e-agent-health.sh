#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../shared/harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-agent-health)"
sa_pick_ports 26000 20000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SA_TEST_NAME="e2e-agent-health"
SA_WAIT_LOGS=("$TMP/agent.log" "$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref AGENT_PID
  sa_kill_pid_ref GATEWAY_PID
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-health] building binaries"
sa_build_go_bins sysarmor-agent sysarmor-gateway sysarmor-manager sysarmorctl

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-health
  host_id: e2e-host
  tenant_id: default
  token: $TOKEN

manager:
  address: 127.0.0.1:$GRPC_PORT
  transport: grpc

sensor:
  backend: fake
  mode: managed
  policy_path: $TMP/policy.yaml
  scope:
    type: container
    selector: e2e-scope
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

telemetry:
  batch_size: 256
  flush_interval: 100ms

data_plane:
  retry_initial: 50ms
  retry_max: 100ms
  request_timeout: 2s

health:
  interval: 100ms
EOF

sa_start_memory_agent_stack "$TOKEN"

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/healthz.json"

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

sa_wait_contains "sysarmorctl agents" "e2e-agent-health" "$RESULTS/e2e-agent-health.agents.json" sa_manager_ctl agents list
sa_wait_contains "sysarmorctl agents tenant" '"tenant_id":"default"' "$RESULTS/e2e-agent-health.agents.json" sa_manager_ctl agents list
sa_wait_contains "sysarmorctl agents health status" '"health_status":"ok"' "$RESULTS/e2e-agent-health.agents.json" sa_manager_ctl agents list
sa_wait_contains "sysarmorctl agents scope" '"scope":{"type":"container","selector":"e2e-scope"}' "$RESULTS/e2e-agent-health.agents.json" sa_manager_ctl agents list
sa_wait_contains "sysarmorctl agents filtered" '"agent_id":"e2e-agent-health"' "$RESULTS/e2e-agent-health.agents-filtered.json" \
  sa_manager_ctl agents list --tenant-id default --scope-type container --scope-selector e2e-scope --health-status ok
sa_wait_contains "sysarmorctl agents filtered scope" '"scope":{"type":"container","selector":"e2e-scope"}' "$RESULTS/e2e-agent-health.agents-filtered.json" \
  sa_manager_ctl agents list --tenant-id default --scope-type container --scope-selector e2e-scope --health-status ok
sa_wait_contains "sysarmorctl agents filtered capability" '"sensor_capability"' "$RESULTS/e2e-agent-health.agents-filtered.json" \
  sa_manager_ctl agents list --tenant-id default --scope-type container --scope-selector e2e-scope --health-status ok
sa_wait_contains "sysarmorctl agent-health" '"agent_id":"e2e-agent-health"' "$RESULTS/e2e-agent-health.health.json" \
  sa_manager_ctl health get --agent-id e2e-agent-health --tenant-id default
sa_wait_contains "sysarmorctl agent-health sensor" '"sensor_health"' "$RESULTS/e2e-agent-health.health.json" \
  sa_manager_ctl health get --agent-id e2e-agent-health --tenant-id default
sa_wait_contains "sysarmorctl agent-health scope type" '"scope":{"type":"container","selector":"e2e-scope"}' "$RESULTS/e2e-agent-health.health.json" \
  sa_manager_ctl health get --agent-id e2e-agent-health --tenant-id default
sa_wait_contains "sysarmorctl metrics" '"events_ingested":1' "$RESULTS/e2e-agent-health.metrics.json" sa_manager_ctl metrics

cp "$TMP/agent.log" "$RESULTS/e2e-agent-health.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-health.manager.log"

echo "[e2e-agent-health] ok"
