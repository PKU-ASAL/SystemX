#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../shared/harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-response-observe)"
sa_pick_ports 42000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
AGENT_ID="response-agent"
HOST_ID="response-host"
SA_TEST_NAME="e2e-response-observe-only"
SA_WAIT_LOGS=("$TMP/agent.log" "$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref AGENT_PID
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-response-observe-only] building binaries"
sa_build_all

sa_start_memory_manager --dev-token "$TOKEN"

wait_contains() {
  sa_wait_contains "$@"
}

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

cat > "$TMP/collection.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: $AGENT_ID
  host_id: $HOST_ID
  tenant_id: default
  token: $TOKEN
manager:
  address: 127.0.0.1:$GRPC_PORT
  transport: grpc
sensor:
  backend: fake
  mode: managed
  policy_path: $TMP/collection.yaml
  observe_only: true
telemetry:
  batch_size: 10
  flush_interval: 1s
data_plane:
  retry_initial: 10ms
  retry_max: 20ms
  request_timeout: 2s
health:
  interval: 200ms
policy:
  refresh_interval: 0s
EOF

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" > "$TMP/agent.log" 2>&1 &
AGENT_PID=$!

wait_contains "agent health" '"agent_id":"response-agent"' "$RESULTS/e2e-response-observe-only.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --tenant-id default --agent-id "$AGENT_ID"

cat > "$TMP/response.json" <<JSON
{
  "response_id": "resp-observe-1",
  "tenant_id": "default",
  "agent_id": "$AGENT_ID",
  "action": "collect",
  "mode": "observe",
  "target": "process:fake",
  "reason": "response_intent=collect recommended_action=collect",
  "actor": "e2e"
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/responses" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/response.json" > "$RESULTS/e2e-response-observe-only.command.json"

wait_contains "agent ack log" 'agent response ack: response=resp-observe-1 action=collect observe_only=true' "$RESULTS/e2e-response-observe-only.agent-ack.txt" \
  grep -F 'agent response ack: response=resp-observe-1 action=collect observe_only=true' "$TMP/agent.log"

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager responses list --tenant-id default --agent-id "$AGENT_ID" > "$RESULTS/e2e-response-observe-only.audit.json"
for want in '"response_id":"resp-observe-1"' '"status":"acked"' '"observe_only":true' '"executed":false'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-response-observe-only.audit.json"; then
    echo "[e2e-response-observe-only][ERROR] audit missing $want" >&2
    cat "$RESULTS/e2e-response-observe-only.audit.json" >&2
    exit 1
  fi
done

cp "$TMP/agent.log" "$RESULTS/e2e-response-observe-only.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-response-observe-only.manager.log"
echo "[e2e-response-observe-only] ok"
