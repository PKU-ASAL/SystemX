#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-incident-attach-evidence.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((54000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
SCENARIO="incident-attach-evidence"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-incident-attach-evidence] building binaries"
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
      echo "[e2e-incident-attach-evidence][ERROR] $name missing $needle" >&2
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
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json incidents --scenario "$SCENARIO"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json incident-evidence-attach \
  --scenario "$SCENARIO" \
  --node-id "user:root" \
  --node-kind user \
  --node-label root \
  --edge-from "process:p-bash" \
  --edge-to "user:root" \
  --edge-kind ran_as > "$RESULTS/e2e-incident-attach-evidence.attach.json"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json incident-evidence \
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
