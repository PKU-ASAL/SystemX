#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-link1-downlink.XXXXXX")"
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

echo "[e2e-link1-downlink] building binaries"
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
      echo "[e2e-link1-downlink][ERROR] $name missing $needle" >&2
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
  "agent_id": "link1-downlink-agent",
  "action": "collect",
  "target": "process:p1",
  "reason": "e2e downlink"
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/responses" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/response.json" > "$RESULTS/e2e-link1-downlink.response.json"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json evidence-pullbacks \
  --create \
  --request-id link1-downlink-pullback \
  --tenant-id default \
  --agent-id link1-downlink-agent \
  --incident-id inc-link1-downlink \
  --target process:p1 \
  --reason "collect process tree" > "$RESULTS/e2e-link1-downlink.pullback.json"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json link1-downlink \
  --tenant-id default \
  --agent-id link1-downlink-agent > "$RESULTS/e2e-link1-downlink.frames.json"

for want in '"type":"policy_update"' '"policy_id":"default-edr-policy"' '"type":"response_command"' '"agent_id":"link1-downlink-agent"' '"action":"collect"' '"type":"evidence_pullback"' '"request_id":"link1-downlink-pullback"' '"target":"process:p1"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-link1-downlink.frames.json"; then
    echo "[e2e-link1-downlink][ERROR] downlink missing $want" >&2
    cat "$RESULTS/e2e-link1-downlink.frames.json" >&2
    exit 1
  fi
done

cp "$TMP/manager.log" "$RESULTS/e2e-link1-downlink.manager.log"
echo "[e2e-link1-downlink] ok"
