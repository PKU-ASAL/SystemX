#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-response-scope-deny.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((46000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
AGENT_ID="response-scope-agent"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-response-scope-deny] building binaries"
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
      echo "[e2e-response-scope-deny][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

cat > "$TMP/agent-health.json" <<JSON
{
  "agent_id": "$AGENT_ID",
  "host_id": "response-scope-host",
  "tenant_id": "default",
  "scope": {"type": "container", "selector": "abc123"},
  "status": "ok",
  "policy_id": "default-edr-policy",
  "policy_version": 1,
  "policy_mode": "observe",
  "sensor_health": {"backend": "fake", "installed": true, "running": true, "version": "test", "policy_loaded": true},
  "observed_at": "2026-06-16T00:00:00Z"
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/agent-health" \
  -H "X-SysArmor-Agent-Token: $TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/agent-health.json" > "$TMP/agent-health.result.json"

cat > "$TMP/scope-deny-command.json" <<JSON
{
  "response_id": "resp-deny-scope",
  "tenant_id": "default",
  "agent_id": "$AGENT_ID",
  "scope": {"type": "container", "selector": "wrong"},
  "action": "collect",
  "mode": "observe",
  "target": "process:fake",
  "reason": "scope deny e2e",
  "actor": "e2e"
}
JSON

status="$(
  curl -sS -o "$RESULTS/e2e-response-scope-deny.create.json" \
    -w '%{http_code}' \
    -X POST "$MGR_URL/api/v1/responses" \
    -H 'Content-Type: application/json' \
    --data-binary @"$TMP/scope-deny-command.json"
)"
if [[ "$status" != "403" ]]; then
  echo "[e2e-response-scope-deny][ERROR] create status = $status, want 403" >&2
  cat "$RESULTS/e2e-response-scope-deny.create.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager responses list --tenant-id default --agent-id "$AGENT_ID" > "$RESULTS/e2e-response-scope-deny.audit.json"
for want in '"response_id":"resp-deny-scope"' '"status":"denied"' 'response command scope does not match agent runtime scope' '"scope":{"type":"container","selector":"wrong"}'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-response-scope-deny.audit.json"; then
    echo "[e2e-response-scope-deny][ERROR] audit missing $want" >&2
    cat "$RESULTS/e2e-response-scope-deny.audit.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager responses list --tenant-id default --agent-id "$AGENT_ID" --pending > "$RESULTS/e2e-response-scope-deny.pending.json"
if grep -Fq 'resp-deny-scope' "$RESULTS/e2e-response-scope-deny.pending.json"; then
  echo "[e2e-response-scope-deny][ERROR] denied command is pending" >&2
  cat "$RESULTS/e2e-response-scope-deny.pending.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-response-scope-deny.manager.log"
echo "[e2e-response-scope-deny] ok"
