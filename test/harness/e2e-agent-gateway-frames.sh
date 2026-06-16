#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-gateway-frames.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((62000 + RANDOM % 1000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-gateway-frames] building binaries"
make -C "$ROOT" build >/dev/null

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store-backend file \
  --store "$TMP/store.json" \
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
      echo "[e2e-agent-gateway-frames][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

cat > "$TMP/response.json" <<JSON
{
  "response_id": "resp-agent-gateway-frame",
  "tenant_id": "default",
  "agent_id": "agent-gateway-frame-agent",
  "action": "collect",
  "target": "process:p1",
  "reason": "e2e frame"
}
JSON
curl -sf -X POST "$MGR_URL/api/v1/responses" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/response.json" > "$RESULTS/e2e-agent-gateway-frames.response.json"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json evidence-pullbacks \
  --create \
  --request-id evpb-agent-gateway-frame \
  --tenant-id default \
  --agent-id agent-gateway-frame-agent \
  --target process:p1 \
  --reason "e2e frame pullback" > "$RESULTS/e2e-agent-gateway-frames.pullback.json"

cat > "$TMP/frames.json" <<JSON
[
  {
    "type": "upload",
    "payload": {
      "batch_id": "agent-gateway-frame-batch",
      "agent": {
        "agent_id": "agent-gateway-frame-agent",
        "host_id": "agent-gateway-frame-host",
        "tenant_id": "default",
        "version": "e2e"
      },
      "events": [
        {"id": "ev-agent-gateway-frame", "scenario": "agent-gateway-frame", "kind": "EVENT_KIND_EXEC"}
      ]
    }
  },
  {
    "type": "health",
    "payload": {
      "agent_id": "agent-gateway-frame-agent",
      "host_id": "agent-gateway-frame-host",
      "tenant_id": "default",
      "status": "ok"
    }
  },
  {
    "type": "ack",
    "payload": {
      "response_id": "resp-agent-gateway-frame",
      "tenant_id": "default",
      "agent_id": "agent-gateway-frame-agent",
      "accepted": true,
      "observe_only": true
    }
  },
  {
    "type": "evidence_pullback_result",
    "payload": {
      "request_id": "evpb-agent-gateway-frame",
      "tenant_id": "default",
      "agent_id": "agent-gateway-frame-agent",
      "ok": true,
      "message": "collected"
    }
  },
  {
    "type": "error",
    "payload": {"message": "synthetic"}
  }
]
JSON

SYSARMOR_DEV_TOKEN="$TOKEN" "$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-gateway-frames \
  --file "$TMP/frames.json" > "$RESULTS/e2e-agent-gateway-frames.frames.json"

for want in '"type":"upload"' '"batch_id":"agent-gateway-frame-batch"' '"type":"health"' '"type":"ack"' '"type":"evidence_pullback_result"' '"request_id":"evpb-agent-gateway-frame"' '"type":"error"' '"ok":true'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-agent-gateway-frames.frames.json"; then
    echo "[e2e-agent-gateway-frames][ERROR] frame response missing $want" >&2
    cat "$RESULTS/e2e-agent-gateway-frames.frames.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-gateway-sessions \
  --tenant-id default \
  --agent-id agent-gateway-frame-agent > "$RESULTS/e2e-agent-gateway-frames.sessions.json"
for want in '"last_ack_cursor":"agent-gateway-frame-batch"' '"transport":"frame"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-agent-gateway-frames.sessions.json"; then
    echo "[e2e-agent-gateway-frames][ERROR] session missing $want" >&2
    cat "$RESULTS/e2e-agent-gateway-frames.sessions.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-health \
  --tenant-id default \
  --agent-id agent-gateway-frame-agent > "$RESULTS/e2e-agent-gateway-frames.health.json"
if ! grep -Fq '"status":"ok"' "$RESULTS/e2e-agent-gateway-frames.health.json"; then
  echo "[e2e-agent-gateway-frames][ERROR] health frame was not applied" >&2
  cat "$RESULTS/e2e-agent-gateway-frames.health.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json responses \
  --tenant-id default \
  --agent-id agent-gateway-frame-agent > "$RESULTS/e2e-agent-gateway-frames.responses.json"
if ! grep -Fq '"ack":{"response_id":"resp-agent-gateway-frame"' "$RESULTS/e2e-agent-gateway-frames.responses.json"; then
  echo "[e2e-agent-gateway-frames][ERROR] ack frame was not persisted" >&2
  cat "$RESULTS/e2e-agent-gateway-frames.responses.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json evidence-pullbacks \
  --tenant-id default \
  --agent-id agent-gateway-frame-agent > "$RESULTS/e2e-agent-gateway-frames.pullbacks.json"
for want in '"request_id":"evpb-agent-gateway-frame"' '"status":"completed"' '"result_ok":true' '"result":"collected"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-agent-gateway-frames.pullbacks.json"; then
    echo "[e2e-agent-gateway-frames][ERROR] pullback result missing $want" >&2
    cat "$RESULTS/e2e-agent-gateway-frames.pullbacks.json" >&2
    exit 1
  fi
done

cp "$TMP/manager.log" "$RESULTS/e2e-agent-gateway-frames.manager.log"
echo "[e2e-agent-gateway-frames] ok"
