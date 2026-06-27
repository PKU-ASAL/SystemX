#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../shared/harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-incident-merge)"
sa_pick_ports 56000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SA_TEST_NAME="e2e-incident-merge"
SA_WAIT_LOGS=("$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-incident-merge] building binaries"
sa_build_all

sa_start_memory_manager --local-ingest --dev-token "$TOKEN"

wait_contains() {
  sa_wait_contains "$@"
}

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

cat > "$TMP/target.json" <<JSON
{
  "header": {
    "batchId": "incident-merge-target-batch",
    "agentId": "incident-merge-agent",
    "hostId": "incident-merge-host",
    "tenantId": "default",
    "signalCount": 1,
    "labels": {"agent_version": "e2e"}
  },
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-merge-target",
        "name": "reverse_shell_pattern",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 80,
        "globalRarity": 1,
        "lineageId": "lin-merge-target",
        "terminal": true,
        "labels": {"scenario": "incident-merge-target"},
        "entities": [
          {"kind": "process", "key": "process:p-target", "role": "subject"},
          {"kind": "socket", "key": "socket:10.66.0.10:443", "role": "object"}
        ]
      }
    }
  ]
}
JSON

cat > "$TMP/source.json" <<JSON
{
  "header": {
    "batchId": "incident-merge-source-batch",
    "agentId": "incident-merge-agent",
    "hostId": "incident-merge-host",
    "tenantId": "default",
    "signalCount": 1,
    "labels": {"agent_version": "e2e"}
  },
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-merge-source",
        "name": "reverse_shell_pattern",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 80,
        "globalRarity": 1,
        "lineageId": "lin-merge-source",
        "terminal": true,
        "labels": {"scenario": "incident-merge-source"},
        "entities": [
          {"kind": "process", "key": "process:p-source", "role": "subject"},
          {"kind": "socket", "key": "socket:10.66.0.20:443", "role": "object"}
        ]
      }
    }
  ]
}
JSON

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/target.json" > "$RESULTS/e2e-incident-merge.target-data_plane.json"

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/source.json" > "$RESULTS/e2e-incident-merge.source-data_plane.json"

wait_contains "target incident" '"id":"inc-00000000000000000001"' "$RESULTS/e2e-incident-merge.target.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list --label scenario=incident-merge-target
wait_contains "source incident" '"id":"inc-00000000000000000002"' "$RESULTS/e2e-incident-merge.source.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list --label scenario=incident-merge-source

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident merge \
  --target-incident-id inc-00000000000000000001 \
  --source-incident-id inc-00000000000000000002 > "$RESULTS/e2e-incident-merge.merge.json"

for want in '"id":"inc-00000000000000000001"' '"lin-merge-target"' '"lin-merge-source"' '"id":"process:p-source"' '"id":"socket:10.66.0.20:443"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-incident-merge.merge.json"; then
    echo "[e2e-incident-merge][ERROR] merge response missing $want" >&2
    cat "$RESULTS/e2e-incident-merge.merge.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list > "$RESULTS/e2e-incident-merge.all.json"
if grep -Fq '"id":"inc-00000000000000000002"' "$RESULTS/e2e-incident-merge.all.json"; then
  echo "[e2e-incident-merge][ERROR] source incident still queryable after merge" >&2
  cat "$RESULTS/e2e-incident-merge.all.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-incident-merge.manager.log"
echo "[e2e-incident-merge] ok"
