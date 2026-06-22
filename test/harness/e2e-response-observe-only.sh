#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-response-observe.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((42000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
AGENT_ID="response-agent"
HOST_ID="response-host"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]]; then kill "$AGENT_PID" 2>/dev/null || true; fi
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-response-observe-only] building binaries"
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
      echo "[e2e-response-observe-only][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      cat "$TMP/agent.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

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
