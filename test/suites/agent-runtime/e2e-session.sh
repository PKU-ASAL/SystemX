#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-session.XXXXXX")"
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

echo "[e2e-agent-session] building binaries"
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
      echo "[e2e-agent-session][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

for batch in agent-session-batch-1 agent-session-batch-2; do
  cat > "$TMP/$batch.json" <<JSON
{
  "header": {
    "batchId": "$batch",
    "agentId": "agent-session-agent",
    "hostId": "agent-session-host",
    "tenantId": "default",
    "eventCount": 1
  },
  "events": [
    {
      "sequence": 1,
      "event": {
        "id": "ev-$batch",
        "agentId": "agent-session-agent",
        "hostId": "agent-session-host",
        "tenantId": "default",
        "labels": {"scenario": "agent-session"},
        "behavior": "process.exec"
      }
    }
  ]
}
JSON
  "$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/$batch.json" > "$RESULTS/e2e-agent-session.$batch.data_plane.json"
done

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager sessions list \
  --tenant-id default \
  --agent-id agent-session-agent > "$RESULTS/e2e-agent-session.sessions.json"

for want in '"agent_id":"agent-session-agent"' '"tenant_id":"default"' '"last_ack_cursor":"agent-session-batch-2"' '"data_transport":"grpc"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-agent-session.sessions.json"; then
    echo "[e2e-agent-session][ERROR] sessions missing $want" >&2
    cat "$RESULTS/e2e-agent-session.sessions.json" >&2
    exit 1
  fi
done
if grep -Fq '"last_ack_cursor":"agent-session-batch-1"' "$RESULTS/e2e-agent-session.sessions.json"; then
  echo "[e2e-agent-session][ERROR] session cursor did not advance" >&2
  cat "$RESULTS/e2e-agent-session.sessions.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager resume get \
  --tenant-id default \
  --agent-id agent-session-agent > "$RESULTS/e2e-agent-session.resume.json"
if ! grep -Fq '"resume_cursor":"agent-session-batch-2"' "$RESULTS/e2e-agent-session.resume.json"; then
  echo "[e2e-agent-session][ERROR] resume cursor did not match latest batch" >&2
  cat "$RESULTS/e2e-agent-session.resume.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-agent-session.manager.log"
echo "[e2e-agent-session] ok"
