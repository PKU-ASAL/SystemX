#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../../shared/harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-policy-publish)"
sa_pick_ports 53000 2000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
AGENT_ID="policy-publish-agent"
SA_TEST_NAME="e2e-policy-publish"
SA_WAIT_LOGS=("$TMP/manager.log")

cleanup() {
  exec 3>&- 2>/dev/null || true
  sa_kill_pid_ref AGENT_PID
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-policy-publish] building binaries"
sa_build_go_bins sysarmor-manager sysarmorctl

sa_start_memory_manager

sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/health.json"

cat > "$TMP/policy.json" <<'JSON'
{
  "policy_id": "draft-policy",
  "version": 2,
  "tenant_id": "default",
  "endpoint_rules": ["download_by_lolbin"],
  "cloud_rules": ["web_shell_chain"],
  "mode": "observe",
  "published": false
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/policies?actor=e2e&reason=draft" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/policy.json" > "$RESULTS/e2e-policy-publish.draft.json"

for want in '"policy_id":"draft-policy"' '"published":false'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-policy-publish.draft.json"; then
    echo "[e2e-policy-publish][ERROR] draft missing $want" >&2
    cat "$RESULTS/e2e-policy-publish.draft.json" >&2
    exit 1
  fi
done

cat > "$TMP/assignment.json" <<JSON
{
  "tenant_id": "default",
  "agent_id": "$AGENT_ID",
  "policy_id": "draft-policy",
  "policy_version": 2,
  "actor": "operator",
  "reason": "deploy"
}
JSON

status="$(
  curl -sS -o "$RESULTS/e2e-policy-publish.assignment-before.json" \
    -w '%{http_code}' \
    -X POST "$MGR_URL/api/v1/policy-assignments" \
    -H 'Content-Type: application/json' \
    --data-binary @"$TMP/assignment.json"
)"
if [[ "$status" != "400" ]]; then
  echo "[e2e-policy-publish][ERROR] draft assignment status = $status, want 400" >&2
  cat "$RESULTS/e2e-policy-publish.assignment-before.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager policies publish \
  --tenant-id default \
  --policy-id draft-policy \
  --version 2 \
  --actor reviewer \
  --reason "ready for assignment" > "$RESULTS/e2e-policy-publish.publish.json"

for want in '"policy_id":"draft-policy"' '"published":true'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-policy-publish.publish.json"; then
    echo "[e2e-policy-publish][ERROR] publish missing $want" >&2
    cat "$RESULTS/e2e-policy-publish.publish.json" >&2
    exit 1
  fi
done

curl -sf -X POST "$MGR_URL/api/v1/policy-assignments" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/assignment.json" > "$RESULTS/e2e-policy-publish.assignment-after.json"

sa_wait_contains "effective policy" '"policy_id":"draft-policy"' "$RESULTS/e2e-policy-publish.effective.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager policies effective --tenant-id default --agent-id "$AGENT_ID"
if ! grep -Fq '"published":true' "$RESULTS/e2e-policy-publish.effective.json"; then
  echo "[e2e-policy-publish][ERROR] effective policy is not published" >&2
  cat "$RESULTS/e2e-policy-publish.effective.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager policies audit \
  --tenant-id default \
  --policy-id draft-policy > "$RESULTS/e2e-policy-publish.audit.json"

for want in '"action":"policy.upsert"' '"actor":"e2e"' '"action":"policy.publish"' '"actor":"reviewer"' '"action":"policy.assign"' '"actor":"operator"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-policy-publish.audit.json"; then
    echo "[e2e-policy-publish][ERROR] audit missing $want" >&2
    cat "$RESULTS/e2e-policy-publish.audit.json" >&2
    exit 1
  fi
done

cp "$TMP/manager.log" "$RESULTS/e2e-policy-publish.manager.log"
echo "[e2e-policy-publish] ok"
