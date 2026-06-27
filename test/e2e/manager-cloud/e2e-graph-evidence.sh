#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../shared/harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-graph-evidence)"
sa_pick_ports 50000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SCENARIO="apt-staged-drop-graph"
SA_TEST_NAME="e2e-graph-evidence"
SA_WAIT_LOGS=("$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-graph-evidence] building binaries"
sa_build_all

sa_start_memory_manager --local-ingest --dev-token "$TOKEN"

wait_contains() {
  sa_wait_contains "$@"
}

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

cat > "$TMP/batch.json" <<JSON
{
  "header": {
    "batchId": "graph-evidence-batch",
    "agentId": "graph-evidence-agent",
    "hostId": "graph-evidence-host",
    "tenantId": "default",
    "signalCount": 2,
    "labels": {"agent_version": "e2e"}
  },
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-graph-payload",
        "name": "payload_dropped",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 45,
        "globalRarity": 1,
        "lineageId": "lin-drop",
        "labels": {"scenario": "$SCENARIO"},
        "entities": [{"kind": "file", "key": "file:/var/lib/app/plugins/helper", "role": "object"}]
      }
    },
    {
      "sequence": 2,
      "signal": {
        "id": "sig-graph-connect",
        "name": "suspicious_exec_connect",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 65,
        "globalRarity": 1,
        "lineageId": "lin-connect",
        "labels": {"scenario": "$SCENARIO"},
        "entities": [
          {"kind": "file", "key": "file:/var/lib/app/plugins/helper", "role": "object"},
          {"kind": "socket", "key": "socket:10.66.0.99:443", "role": "object"}
        ]
      }
    }
  ]
}
JSON

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-graph-evidence.data_plane.json"

wait_contains "incident" '"incidents":[{' "$RESULTS/e2e-graph-evidence.incidents.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list --label scenario="$SCENARIO"

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident evidence --label scenario="$SCENARIO" > "$RESULTS/e2e-graph-evidence.evidence.json"
for want in '"id":"file:/var/lib/app/plugins/helper"' '"id":"socket:10.66.0.99:443"' '"from":"file:/var/lib/app/plugins/helper"' '"to":"socket:10.66.0.99:443"' '"kind":"connect"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-graph-evidence.evidence.json"; then
    echo "[e2e-graph-evidence][ERROR] evidence missing $want" >&2
    cat "$RESULTS/e2e-graph-evidence.evidence.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident evidence \
  --label scenario="$SCENARIO" \
  --path-from "file:/var/lib/app/plugins/helper" \
  --path-to "socket:10.66.0.99:443" > "$RESULTS/e2e-graph-evidence.path.json"
for want in '"id":"file:/var/lib/app/plugins/helper"' '"id":"socket:10.66.0.99:443"' '"kind":"connect"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-graph-evidence.path.json"; then
    echo "[e2e-graph-evidence][ERROR] path missing $want" >&2
    cat "$RESULTS/e2e-graph-evidence.path.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident evidence \
  --label scenario="$SCENARIO" \
  --seed "file:/var/lib/app/plugins/helper" \
  --hops 1 > "$RESULTS/e2e-graph-evidence.khop.json"
if ! grep -Fq '"id":"socket:10.66.0.99:443"' "$RESULTS/e2e-graph-evidence.khop.json"; then
  echo "[e2e-graph-evidence][ERROR] k-hop neighborhood missing socket node" >&2
  cat "$RESULTS/e2e-graph-evidence.khop.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-graph-evidence.manager.log"
echo "[e2e-graph-evidence] ok"
