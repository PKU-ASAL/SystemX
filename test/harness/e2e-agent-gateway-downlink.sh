#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-gateway-downlink.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((62000 + RANDOM % 1000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-gateway-downlink] building binaries"
make -C "$ROOT" build >/dev/null

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store-backend file \
  --store "$TMP/store.json" \
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
      echo "[e2e-agent-gateway-downlink][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

cat > "$TMP/response.json" <<JSON
{
  "tenant_id": "default",
  "agent_id": "agent-gateway-downlink-agent",
  "action": "collect",
  "target": "process:p1",
  "reason": "e2e downlink"
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/responses" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/response.json" > "$RESULTS/e2e-agent-gateway-downlink.response.json"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json evidence-pullbacks \
  --create \
  --request-id agent-gateway-downlink-pullback \
  --tenant-id default \
  --agent-id agent-gateway-downlink-agent \
  --incident-id inc-agent-gateway-downlink \
  --target process:p1 \
  --reason "collect process tree" > "$RESULTS/e2e-agent-gateway-downlink.pullback.json"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-gateway-downlink \
  --tenant-id default \
  --agent-id agent-gateway-downlink-agent > "$RESULTS/e2e-agent-gateway-downlink.frames.json"

for want in '"type":"resume"' '"type":"policy_update"' '"policy_id":"default-edr-policy"' '"type":"response_command"' '"agent_id":"agent-gateway-downlink-agent"' '"action":"collect"' '"type":"evidence_pullback"' '"request_id":"agent-gateway-downlink-pullback"' '"target":"process:p1"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-agent-gateway-downlink.frames.json"; then
    echo "[e2e-agent-gateway-downlink][ERROR] downlink missing $want" >&2
    cat "$RESULTS/e2e-agent-gateway-downlink.frames.json" >&2
    exit 1
  fi
done

cp "$TMP/manager.log" "$RESULTS/e2e-agent-gateway-downlink.manager.log"
echo "[e2e-agent-gateway-downlink] ok"
