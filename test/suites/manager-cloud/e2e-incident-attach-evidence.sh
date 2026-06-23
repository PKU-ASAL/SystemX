#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-incident-attach-evidence)"
sa_pick_ports 54000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SCENARIO="incident-attach-evidence"
SA_TEST_NAME="e2e-incident-attach-evidence"
SA_WAIT_LOGS=("$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-incident-attach-evidence] building binaries"
sa_build_all

sa_start_memory_manager --local-ingest --dev-token "$TOKEN"

wait_contains() {
  sa_wait_contains "$@"
}

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

cat > "$TMP/batch.json" <<JSON
{
  "header": {
    "batchId": "incident-attach-evidence-batch",
    "agentId": "incident-attach-evidence-agent",
    "hostId": "incident-attach-evidence-host",
    "tenantId": "default",
    "signalCount": 2,
    "labels": {"agent_version": "e2e"}
  },
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-attach-web",
        "name": "web_runtime_spawns_shell",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 50,
        "globalRarity": 1,
        "lineageId": "lin-attach",
        "scenario": "$SCENARIO",
        "entities": [{"kind": "process", "key": "process:p-web", "role": "subject"}]
      }
    },
    {
      "sequence": 2,
      "signal": {
        "id": "sig-attach-rev",
        "name": "reverse_shell_pattern",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 80,
        "globalRarity": 1,
        "lineageId": "lin-attach",
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

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-incident-attach-evidence.data_plane.json"

wait_contains "incident" '"status":"open"' "$RESULTS/e2e-incident-attach-evidence.incidents.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list --scenario "$SCENARIO"

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident evidence attach \
  --scenario "$SCENARIO" \
  --node-id "user:root" \
  --node-kind user \
  --node-label root \
  --edge-from "process:p-bash" \
  --edge-to "user:root" \
  --edge-kind ran_as > "$RESULTS/e2e-incident-attach-evidence.attach.json"

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident evidence \
  --scenario "$SCENARIO" > "$RESULTS/e2e-incident-attach-evidence.evidence.json"

for want in '"id":"user:root"' '"kind":"user"' '"from":"process:p-bash"' '"to":"user:root"' '"kind":"ran_as"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-incident-attach-evidence.evidence.json"; then
    echo "[e2e-incident-attach-evidence][ERROR] evidence missing $want" >&2
    cat "$RESULTS/e2e-incident-attach-evidence.evidence.json" >&2
    exit 1
  fi
done

cp "$TMP/manager.log" "$RESULTS/e2e-incident-attach-evidence.manager.log"
echo "[e2e-incident-attach-evidence] ok"
