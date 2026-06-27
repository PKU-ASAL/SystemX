#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../shared/harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-response-audit)"
sa_pick_ports 48000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
AGENT_ID="response-audit-agent"
SA_TEST_NAME="e2e-response-audit"
SA_WAIT_LOGS=("$TMP/manager.log")

cleanup() {
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-response-audit] building binaries"
sa_build_all

sa_start_memory_manager --local-ingest --dev-token "$TOKEN"

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

cat > "$TMP/batch.json" <<JSON
{
  "header": {
    "batchId": "response-audit-batch",
    "agentId": "$AGENT_ID",
    "hostId": "response-audit-host",
    "tenantId": "default",
    "signalCount": 1,
    "labels": {"agent_version": "e2e"}
  },
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-response-intent",
        "name": "reverse_shell_pattern",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 80,
        "globalRarity": 1,
        "lineageId": "lin-response-audit",
        "terminal": true,
        "labels": {"scenario": "response-audit"},
        "responseIntent": {
          "responseIntent": "collect",
          "recommendedAction": "collect",
          "confidence": 80,
          "reason": "terminal reverse shell pattern"
        },
        "entities": [
          {"kind": "process", "key": "process:p-bash", "role": "subject"},
          {"kind": "socket", "key": "socket:10.66.0.99:443", "role": "object"}
        ]
      }
    }
  ]
}
JSON

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-response-audit.data_plane.json"

sa_wait_contains "signal intent" '"response_intent":{"response_intent":"collect","recommended_action":"collect","confidence":80,"reason":"terminal reverse shell pattern"}' "$RESULTS/e2e-response-audit.signals.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager signals list --label scenario=response-audit --terminal

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager response decide \
  --tenant-id default \
  --agent-id "$AGENT_ID" \
  --signal-id sig-response-intent \
  --actor e2e > "$RESULTS/e2e-response-audit.decision.json"

for want in '"response_id":"resp-sig-response-intent"' '"signal_id":"sig-response-intent"' '"action":"collect"' '"mode":"observe"' '"status":"pending"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-response-audit.decision.json"; then
    echo "[e2e-response-audit][ERROR] decision missing $want" >&2
    cat "$RESULTS/e2e-response-audit.decision.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager responses list --tenant-id default --agent-id "$AGENT_ID" > "$RESULTS/e2e-response-audit.audit.json"
for want in '"response_id":"resp-sig-response-intent"' '"signal_id":"sig-response-intent"' 'response_intent=collect confidence=80'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-response-audit.audit.json"; then
    echo "[e2e-response-audit][ERROR] audit missing $want" >&2
    cat "$RESULTS/e2e-response-audit.audit.json" >&2
    exit 1
  fi
done

cp "$TMP/manager.log" "$RESULTS/e2e-response-audit.manager.log"
echo "[e2e-response-audit] ok"
