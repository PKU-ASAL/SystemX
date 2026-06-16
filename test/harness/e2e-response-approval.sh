#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-response-approval.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((50000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
AGENT_ID="response-approval-agent"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-response-approval] building binaries"
make -C "$ROOT" build >/dev/null

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
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
      echo "[e2e-response-approval][ERROR] $name missing $needle" >&2
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
  "response_id": "resp-approval-collect",
  "tenant_id": "default",
  "agent_id": "$AGENT_ID",
  "action": "collect",
  "mode": "observe",
  "target": "process:approval",
  "reason": "approval required e2e",
  "actor": "e2e",
  "approval_required": true
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/responses" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/response.json" > "$RESULTS/e2e-response-approval.create.json"

for want in '"response_id":"resp-approval-collect"' '"status":"pending_approval"' '"approval_required":true' '"approval_status":"required"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-response-approval.create.json"; then
    echo "[e2e-response-approval][ERROR] create missing $want" >&2
    cat "$RESULTS/e2e-response-approval.create.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json responses --tenant-id default --agent-id "$AGENT_ID" --pending > "$RESULTS/e2e-response-approval.pending-before.json"
if grep -Fq 'resp-approval-collect' "$RESULTS/e2e-response-approval.pending-before.json"; then
  echo "[e2e-response-approval][ERROR] pending_approval command is pending before approval" >&2
  cat "$RESULTS/e2e-response-approval.pending-before.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json response-approval \
  --tenant-id default \
  --agent-id "$AGENT_ID" \
  --response-id resp-approval-collect \
  --actor analyst \
  --reason "approved for evidence collection" > "$RESULTS/e2e-response-approval.approve.json"

for want in '"response_id":"resp-approval-collect"' '"status":"pending"' '"approval_status":"approved"' '"approved_by":"analyst"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-response-approval.approve.json"; then
    echo "[e2e-response-approval][ERROR] approval missing $want" >&2
    cat "$RESULTS/e2e-response-approval.approve.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json responses --tenant-id default --agent-id "$AGENT_ID" --pending > "$RESULTS/e2e-response-approval.pending-after.json"
if ! grep -Fq '"response_id":"resp-approval-collect"' "$RESULTS/e2e-response-approval.pending-after.json"; then
  echo "[e2e-response-approval][ERROR] approved command is not pending" >&2
  cat "$RESULTS/e2e-response-approval.pending-after.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-response-approval.manager.log"
echo "[e2e-response-approval] ok"
