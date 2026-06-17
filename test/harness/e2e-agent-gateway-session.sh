#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-gateway-session.XXXXXX")"
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

echo "[e2e-agent-gateway-session] building binaries"
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
      echo "[e2e-agent-gateway-session][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

for batch in agent-gateway-session-batch-1 agent-gateway-session-batch-2; do
  cat > "$TMP/$batch.json" <<JSON
{
  "batch_id": "$batch",
  "agent": {
    "agent_id": "agent-gateway-session-agent",
    "host_id": "agent-gateway-session-host",
    "tenant_id": "default",
    "version": "e2e"
  },
  "events": [
    {"id": "ev-$batch", "scenario": "agent-gateway-session", "behavior": "process.exec"}
  ]
}
JSON
  curl -sf -X POST "$MGR_URL/api/v1/upload" \
    -H "X-SysArmor-Agent-Token: $TOKEN" \
    -H 'Content-Type: application/json' \
    --data-binary @"$TMP/$batch.json" > "$RESULTS/e2e-agent-gateway-session.$batch.upload.json"
done

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-gateway-sessions \
  --tenant-id default \
  --agent-id agent-gateway-session-agent > "$RESULTS/e2e-agent-gateway-session.sessions.json"

for want in '"agent_id":"agent-gateway-session-agent"' '"tenant_id":"default"' '"last_ack_cursor":"agent-gateway-session-batch-2"' '"transport":"http"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-agent-gateway-session.sessions.json"; then
    echo "[e2e-agent-gateway-session][ERROR] sessions missing $want" >&2
    cat "$RESULTS/e2e-agent-gateway-session.sessions.json" >&2
    exit 1
  fi
done
if grep -Fq '"last_ack_cursor":"agent-gateway-session-batch-1"' "$RESULTS/e2e-agent-gateway-session.sessions.json"; then
  echo "[e2e-agent-gateway-session][ERROR] session cursor did not advance" >&2
  cat "$RESULTS/e2e-agent-gateway-session.sessions.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-gateway-resume \
  --tenant-id default \
  --agent-id agent-gateway-session-agent > "$RESULTS/e2e-agent-gateway-session.resume.json"
if ! grep -Fq '"resume_cursor":"agent-gateway-session-batch-2"' "$RESULTS/e2e-agent-gateway-session.resume.json"; then
  echo "[e2e-agent-gateway-session][ERROR] resume cursor did not match latest batch" >&2
  cat "$RESULTS/e2e-agent-gateway-session.resume.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-agent-gateway-session.manager.log"
echo "[e2e-agent-gateway-session] ok"
