#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-response-audit.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((48000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
AGENT_ID="response-audit-agent"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-response-audit] building binaries"
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
      echo "[e2e-response-audit][ERROR] $name missing $needle" >&2
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
        "scenario": "response-audit",
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

"$BIN/sysarmor-databatch-upload" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-response-audit.upload.json"

wait_contains "signal intent" '"response_intent":{"response_intent":"collect","recommended_action":"collect","confidence":80,"reason":"terminal reverse shell pattern"}' "$RESULTS/e2e-response-audit.signals.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json signals --scenario response-audit --terminal

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json response-decision \
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

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json responses --tenant-id default --agent-id "$AGENT_ID" > "$RESULTS/e2e-response-audit.audit.json"
for want in '"response_id":"resp-sig-response-intent"' '"signal_id":"sig-response-intent"' 'response_intent=collect confidence=80'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-response-audit.audit.json"; then
    echo "[e2e-response-audit][ERROR] audit missing $want" >&2
    cat "$RESULTS/e2e-response-audit.audit.json" >&2
    exit 1
  fi
done

cp "$TMP/manager.log" "$RESULTS/e2e-response-audit.manager.log"
echo "[e2e-response-audit] ok"
