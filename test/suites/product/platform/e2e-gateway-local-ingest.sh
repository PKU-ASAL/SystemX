#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../../shared/harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-gateway-local-ingest)"
sa_pick_ports 45000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SA_TEST_NAME="e2e-gateway-local-ingest"
SA_WAIT_LOGS=("$TMP/agent.log" "$TMP/gateway.log")

cleanup() {
  sa_kill_pid_ref AGENT_PID
  sa_kill_pid_ref GATEWAY_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-gateway-local-ingest] building binaries"
sa_build_go_bins sysarmor-agent sysarmor-gateway

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-gateway-local-ingest
  host_id: e2e-host
  tenant_id: default
  token: $TOKEN

manager:
  address: 127.0.0.1:$GRPC_PORT
  transport: grpc

control:
  socket_path: $TMP/agent.sock

content:
  path: $TMP/content

sensor:
  backend: fake
  mode: managed
  observe_only: true

telemetry:
  max_batch_items: 1
  flush_interval: 100ms

local:
  state_path: $TMP/state
  export:
    retry_initial: 50ms
    retry_max: 100ms
    request_timeout: 2s

policy:
  path: $TMP/policy.yaml

health:
  interval: 100ms
EOF

sa_start_memory_gateway --local-ingest --dev-token "$TOKEN"
sa_wait_url_contains "$GATEWAY_URL/healthz" '"ok":true' "$TMP/gateway-health.json"

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

sa_wait_url_contains "$GATEWAY_URL/metrics" '"accepted_batches":' "$RESULTS/e2e-gateway-local-ingest.metrics.json"
sa_wait_contains "gateway accepted events" '"accepted_events":1' "$RESULTS/e2e-gateway-local-ingest.metrics.json" curl -sf "$GATEWAY_URL/metrics"

cp "$TMP/agent.log" "$RESULTS/e2e-gateway-local-ingest.agent.log"
cp "$TMP/gateway.log" "$RESULTS/e2e-gateway-local-ingest.gateway.log"

echo "[e2e-gateway-local-ingest] ok"
