#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-policy-cloud-disable)"
sa_pick_ports 36000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SCENARIO="apt-staged-drop-policy"
SA_TEST_NAME="e2e-policy-cloud-disable"
SA_WAIT_LOGS=("$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-policy-cloud-disable] building binaries"
sa_build_all

sa_start_memory_manager --local-ingest --dev-token "$TOKEN"

wait_contains() {
  sa_wait_contains "$@"
}

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

cat > "$TMP/no-cross-policy.json" <<'JSON'
{
  "policy_id": "no-cross-incident",
  "version": 1,
  "tenant_id": "default",
  "endpoint_rules": [
    "web_runtime_spawns_shell",
    "download_by_lolbin",
    "payload_dropped",
    "reverse_shell_pattern",
    "suspicious_exec_connect"
  ],
  "cloud_rules": ["web_shell_chain"],
  "mode": "observe",
  "converge": {
    "mode": "rarity_structural",
    "top_k": 8,
    "max_path_hops": 6,
    "cross_lineage": true
  },
  "published": true
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/policies" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/no-cross-policy.json" > "$RESULTS/e2e-policy-cloud-disable.policy.json"

cat > "$TMP/assignment.json" <<'JSON'
{
  "tenant_id": "default",
  "agent_id": "policy-agent",
  "policy_id": "no-cross-incident"
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/policy-assignments" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/assignment.json" > "$RESULTS/e2e-policy-cloud-disable.assignment.json"

wait_contains "effective policy" '"policy_id":"no-cross-incident"' "$RESULTS/e2e-policy-cloud-disable.effective.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager policies effective --tenant-id default --agent-id policy-agent

cat > "$TMP/batch.json" <<EOF
{
  "header": {
    "batchId": "policy-cloud-disable-1",
    "agentId": "policy-agent",
    "hostId": "policy-host",
    "tenantId": "default",
    "signalCount": 2,
    "labels": {"agent_version": "e2e"}
  },
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "name": "payload_dropped",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 50,
        "globalRarity": 1,
        "lineageId": "lin-drop",
        "labels": {"scenario": "$SCENARIO"},
        "entities": [{"kind": "file", "key": "file:/var/lib/app/plugins/helper", "role": "object"}]
      }
    },
    {
      "sequence": 2,
      "signal": {
        "name": "suspicious_exec_connect",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 50,
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
EOF

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-policy-cloud-disable.ack.json"

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager signals list --label scenario="$SCENARIO" --layer cloud > "$RESULTS/e2e-policy-cloud-disable.cloud-signals.json"
if grep -Fq 'dropped_payload_executed_and_connects' "$RESULTS/e2e-policy-cloud-disable.cloud-signals.json"; then
  echo "[e2e-policy-cloud-disable][ERROR] disabled cloud rule still emitted signal" >&2
  cat "$RESULTS/e2e-policy-cloud-disable.cloud-signals.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list --label scenario="$SCENARIO" > "$RESULTS/e2e-policy-cloud-disable.incidents.json"
if grep -Fq '"inc-' "$RESULTS/e2e-policy-cloud-disable.incidents.json"; then
  echo "[e2e-policy-cloud-disable][ERROR] disabled cloud rule still created incident" >&2
  cat "$RESULTS/e2e-policy-cloud-disable.incidents.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager policies assignments --tenant-id default --agent-id policy-agent > "$RESULTS/e2e-policy-cloud-disable.assignments.json"
if ! grep -Fq '"policy_id":"no-cross-incident"' "$RESULTS/e2e-policy-cloud-disable.assignments.json"; then
  echo "[e2e-policy-cloud-disable][ERROR] assignment not queryable" >&2
  cat "$RESULTS/e2e-policy-cloud-disable.assignments.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-policy-cloud-disable.manager.log"
echo "[e2e-policy-cloud-disable] ok"
