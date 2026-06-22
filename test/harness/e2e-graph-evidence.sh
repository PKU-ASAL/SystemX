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
        "scenario": "$SCENARIO",
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
        "scenario": "$SCENARIO",
        "entities": [
          {"kind": "file", "key": "file:/var/lib/app/plugins/helper", "role": "object"},
          {"kind": "socket", "key": "socket:10.66.0.99:443", "role": "object"}
        ]
      }
    }
  ]
}
JSON

"$BIN/sysarmor-databatch-upload" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/batch.json" > "$RESULTS/e2e-graph-evidence.upload.json"

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

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json incident-evidence \
  --scenario "$SCENARIO" \
  --path-from "file:/var/lib/app/plugins/helper" \
  --path-to "socket:10.66.0.99:443" > "$RESULTS/e2e-graph-evidence.path.json"
for want in '"id":"file:/var/lib/app/plugins/helper"' '"id":"socket:10.66.0.99:443"' '"kind":"connect"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-graph-evidence.path.json"; then
    echo "[e2e-graph-evidence][ERROR] path missing $want" >&2
    cat "$RESULTS/e2e-graph-evidence.path.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json incident-evidence \
  --scenario "$SCENARIO" \
  --seed "file:/var/lib/app/plugins/helper" \
  --hops 1 > "$RESULTS/e2e-graph-evidence.khop.json"
if ! grep -Fq '"id":"socket:10.66.0.99:443"' "$RESULTS/e2e-graph-evidence.khop.json"; then
  echo "[e2e-graph-evidence][ERROR] k-hop neighborhood missing socket node" >&2
  cat "$RESULTS/e2e-graph-evidence.khop.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-graph-evidence.manager.log"
echo "[e2e-graph-evidence] ok"
