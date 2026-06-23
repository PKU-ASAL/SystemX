#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-incident-lifecycle)"
sa_pick_ports 52000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SCENARIO="incident-lifecycle"
SA_TEST_NAME="e2e-incident-lifecycle"
SA_WAIT_LOGS=("$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-incident-lifecycle] building binaries"
sa_build_all

sa_start_memory_manager --local-ingest --dev-token "$TOKEN"

wait_contains() {
  sa_wait_contains "$@"
}

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

cat > "$TMP/batch.json" <<JSON
{
  "header": {
    "batchId": "incident-lifecycle-batch",
    "agentId": "incident-lifecycle-agent",
    "hostId": "incident-lifecycle-host",
    "tenantId": "default",
    "signalCount": 2,
    "labels": {"agent_version": "e2e"}
  },
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-lifecycle-web",
        "name": "web_runtime_spawns_shell",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 50,
        "globalRarity": 1,
        "lineageId": "lin-life",
        "scenario": "$SCENARIO",
        "entities": [{"kind": "process", "key": "process:p-web", "role": "subject"}]
      }
    },
    {
      "sequence": 2,
      "signal": {
        "id": "sig-lifecycle-rev",
        "name": "reverse_shell_pattern",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 80,
        "globalRarity": 1,
        "lineageId": "lin-life",
        "terminal": true,
        "scenario": "$SCENARIO",
        "entities": [
          {"kind": "process", "key": "process:p-bash", "role": "subject"},
          {"kind": "socket", "key": "socket:10.66.0.99:443", "role": "object"}
        ]
      }
    }
  ]
}
JSON

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-incident-lifecycle.data_plane.json"

wait_contains "incident open" '"status":"open"' "$RESULTS/e2e-incident-lifecycle.open.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list --scenario "$SCENARIO"

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident lifecycle \
  --scenario "$SCENARIO" \
  --status suppressed \
  --reason "known drill" \
  --actor e2e > "$RESULTS/e2e-incident-lifecycle.suppress.json"

for want in '"status":"suppressed"' '"status_reason":"known drill"' '"status_actor":"e2e"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-incident-lifecycle.suppress.json"; then
    echo "[e2e-incident-lifecycle][ERROR] suppress missing $want" >&2
    cat "$RESULTS/e2e-incident-lifecycle.suppress.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident lifecycle \
  --scenario "$SCENARIO" \
  --status closed \
  --reason "triaged" \
  --actor e2e > "$RESULTS/e2e-incident-lifecycle.close.json"

for want in '"status":"closed"' '"status_reason":"triaged"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-incident-lifecycle.close.json"; then
    echo "[e2e-incident-lifecycle][ERROR] close missing $want" >&2
    cat "$RESULTS/e2e-incident-lifecycle.close.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident lifecycle \
  --scenario "$SCENARIO" \
  --status open \
  --reason "reopened" \
  --actor e2e > "$RESULTS/e2e-incident-lifecycle.reopen.json"

if ! grep -Fq '"status":"open"' "$RESULTS/e2e-incident-lifecycle.reopen.json"; then
  echo "[e2e-incident-lifecycle][ERROR] reopen did not restore open status" >&2
  cat "$RESULTS/e2e-incident-lifecycle.reopen.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-incident-lifecycle.manager.log"
echo "[e2e-incident-lifecycle] ok"
