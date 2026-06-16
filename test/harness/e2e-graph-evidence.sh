#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-graph-evidence.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((50000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
SCENARIO="apt-staged-drop-graph"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-graph-evidence] building binaries"
make -C "$ROOT" build >/dev/null

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
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
      echo "[e2e-graph-evidence][ERROR] $name missing $needle" >&2
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
  "batch_id": "graph-evidence-batch",
  "agent": {
    "agent_id": "graph-evidence-agent",
    "host_id": "graph-evidence-host",
    "tenant_id": "default",
    "version": "e2e"
  },
  "signals": [
    {
      "id": "sig-graph-payload",
      "name": "payload_dropped",
      "where": "SIGNAL_WHERE_ENDPOINT",
      "base_risk": 45,
      "global_rarity": 1,
      "lineage_id": "lin-drop",
      "scenario": "$SCENARIO",
      "entities": [
        {"kind": "file", "key": "file:/var/lib/app/plugins/helper", "role": "object"}
      ]
    },
    {
      "id": "sig-graph-connect",
      "name": "suspicious_exec_connect",
      "where": "SIGNAL_WHERE_ENDPOINT",
      "base_risk": 65,
      "global_rarity": 1,
      "lineage_id": "lin-connect",
      "scenario": "$SCENARIO",
      "entities": [
        {"kind": "file", "key": "file:/var/lib/app/plugins/helper", "role": "object"},
        {"kind": "socket", "key": "socket:10.66.0.99:443", "role": "object"}
      ]
    }
  ]
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/upload" \
  -H "X-SysArmor-Agent-Token: $TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/batch.json" > "$RESULTS/e2e-graph-evidence.upload.json"

wait_contains "incident" '"incidents":[{' "$RESULTS/e2e-graph-evidence.incidents.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json incidents --scenario "$SCENARIO"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json incident-evidence --scenario "$SCENARIO" > "$RESULTS/e2e-graph-evidence.evidence.json"
for want in '"id":"file:/var/lib/app/plugins/helper"' '"id":"socket:10.66.0.99:443"' '"from":"file:/var/lib/app/plugins/helper"' '"to":"socket:10.66.0.99:443"' '"kind":"connect"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-graph-evidence.evidence.json"; then
    echo "[e2e-graph-evidence][ERROR] evidence missing $want" >&2
    cat "$RESULTS/e2e-graph-evidence.evidence.json" >&2
    exit 1
  fi
done

cp "$TMP/manager.log" "$RESULTS/e2e-graph-evidence.manager.log"
echo "[e2e-graph-evidence] ok"
