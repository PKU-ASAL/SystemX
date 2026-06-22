#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-query-pagination.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((60000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
SCENARIO="query-pagination"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-query-pagination] building binaries"
make -C "$ROOT" build >/dev/null

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store-backend memory \
  --local-ingest \
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
      echo "[e2e-query-pagination][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

cat > "$TMP/batch.json" <<JSON
{
  "header": {
    "batchId": "query-pagination-batch",
    "agentId": "query-pagination-agent",
    "hostId": "query-pagination-host",
    "tenantId": "default",
    "eventCount": 3,
    "signalCount": 2,
    "labels": {"agent_version": "e2e"}
  },
  "events": [
    {"sequence": 1, "event": {"id": "ev-page-1", "agentId": "query-pagination-agent", "hostId": "query-pagination-host", "tenantId": "default", "scenario": "$SCENARIO", "behavior": "process.exec"}},
    {"sequence": 2, "event": {"id": "ev-page-2", "agentId": "query-pagination-agent", "hostId": "query-pagination-host", "tenantId": "default", "scenario": "$SCENARIO", "behavior": "file.open"}},
    {"sequence": 3, "event": {"id": "ev-page-3", "agentId": "query-pagination-agent", "hostId": "query-pagination-host", "tenantId": "default", "scenario": "$SCENARIO", "behavior": "network.connect"}}
  ],
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-page-1",
        "name": "payload_dropped",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 40,
        "globalRarity": 1,
        "lineageId": "lin-page-1",
        "scenario": "$SCENARIO",
        "entities": [{"kind": "file", "key": "file:/tmp/a", "role": "object"}]
      }
    },
    {
      "sequence": 2,
      "signal": {
        "id": "sig-page-2",
        "name": "reverse_shell_pattern",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 80,
        "globalRarity": 1,
        "lineageId": "lin-page-2",
        "terminal": true,
        "scenario": "$SCENARIO",
        "entities": [{"kind": "process", "key": "process:p-bash", "role": "subject"}]
      }
    }
  ]
}
JSON

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-query-pagination.data_plane.json"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json events \
  --scenario "$SCENARIO" --limit 1 --offset 1 > "$RESULTS/e2e-query-pagination.events.json"
if grep -Fq '"id":"ev-page-1"' "$RESULTS/e2e-query-pagination.events.json" || ! grep -Fq '"id":"ev-page-2"' "$RESULTS/e2e-query-pagination.events.json" || grep -Fq '"id":"ev-page-3"' "$RESULTS/e2e-query-pagination.events.json"; then
  echo "[e2e-query-pagination][ERROR] events page mismatch" >&2
  cat "$RESULTS/e2e-query-pagination.events.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json signals \
  --scenario "$SCENARIO" --limit 1 --offset 1 > "$RESULTS/e2e-query-pagination.signals.json"
if grep -Fq '"id":"sig-page-1"' "$RESULTS/e2e-query-pagination.signals.json" || ! grep -Fq '"id":"sig-page-2"' "$RESULTS/e2e-query-pagination.signals.json"; then
  echo "[e2e-query-pagination][ERROR] signals page mismatch" >&2
  cat "$RESULTS/e2e-query-pagination.signals.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-query-pagination.manager.log"
echo "[e2e-query-pagination] ok"
