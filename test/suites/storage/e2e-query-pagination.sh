#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-query-pagination)"
sa_pick_ports 60000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SCENARIO="query-pagination"
SA_TEST_NAME="e2e-query-pagination"
SA_WAIT_LOGS=("$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-query-pagination] building binaries"
sa_build_all

sa_start_memory_manager --local-ingest --dev-token "$TOKEN"

wait_contains() {
  sa_wait_contains "$@"
}

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

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

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager events list \
  --scenario "$SCENARIO" --limit 1 --offset 1 > "$RESULTS/e2e-query-pagination.events.json"
if grep -Fq '"id":"ev-page-1"' "$RESULTS/e2e-query-pagination.events.json" || ! grep -Fq '"id":"ev-page-2"' "$RESULTS/e2e-query-pagination.events.json" || grep -Fq '"id":"ev-page-3"' "$RESULTS/e2e-query-pagination.events.json"; then
  echo "[e2e-query-pagination][ERROR] events page mismatch" >&2
  cat "$RESULTS/e2e-query-pagination.events.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager signals list \
  --scenario "$SCENARIO" --limit 1 --offset 1 > "$RESULTS/e2e-query-pagination.signals.json"
if grep -Fq '"id":"sig-page-1"' "$RESULTS/e2e-query-pagination.signals.json" || ! grep -Fq '"id":"sig-page-2"' "$RESULTS/e2e-query-pagination.signals.json"; then
  echo "[e2e-query-pagination][ERROR] signals page mismatch" >&2
  cat "$RESULTS/e2e-query-pagination.signals.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-query-pagination.manager.log"
echo "[e2e-query-pagination] ok"
