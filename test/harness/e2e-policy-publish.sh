#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-policy-publish.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((53000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
AGENT_ID="policy-publish-agent"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-policy-publish] building binaries"
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
      echo "[e2e-policy-publish][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

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

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json policy-publish \
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

wait_contains "effective policy" '"policy_id":"draft-policy"' "$RESULTS/e2e-policy-publish.effective.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json effective-policy --tenant-id default --agent-id "$AGENT_ID"
if ! grep -Fq '"published":true' "$RESULTS/e2e-policy-publish.effective.json"; then
  echo "[e2e-policy-publish][ERROR] effective policy is not published" >&2
  cat "$RESULTS/e2e-policy-publish.effective.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json policy-audit \
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
