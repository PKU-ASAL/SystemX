#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-incident-merge.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((56000 + RANDOM % 2000))}"
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

echo "[e2e-incident-merge] building binaries"
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
      echo "[e2e-incident-merge][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

cat > "$TMP/target.json" <<JSON
{
  "header": {
    "batchId": "incident-merge-target-batch",
    "agentId": "incident-merge-agent",
    "hostId": "incident-merge-host",
    "tenantId": "default",
    "signalCount": 1,
    "labels": {"agent_version": "e2e"}
  },
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-merge-target",
        "name": "reverse_shell_pattern",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 80,
        "globalRarity": 1,
        "lineageId": "lin-merge-target",
        "terminal": true,
        "scenario": "incident-merge-target",
        "entities": [
          {"kind": "process", "key": "process:p-target", "role": "subject"},
          {"kind": "socket", "key": "socket:10.66.0.10:443", "role": "object"}
        ]
      }
    }
  ]
}
JSON

cat > "$TMP/source.json" <<JSON
{
  "header": {
    "batchId": "incident-merge-source-batch",
    "agentId": "incident-merge-agent",
    "hostId": "incident-merge-host",
    "tenantId": "default",
    "signalCount": 1,
    "labels": {"agent_version": "e2e"}
  },
  "signals": [
    {
      "sequence": 1,
      "signal": {
        "id": "sig-merge-source",
        "name": "reverse_shell_pattern",
        "where": "SIGNAL_WHERE_ENDPOINT",
        "baseRisk": 80,
        "globalRarity": 1,
        "lineageId": "lin-merge-source",
        "terminal": true,
        "scenario": "incident-merge-source",
        "entities": [
          {"kind": "process", "key": "process:p-source", "role": "subject"},
          {"kind": "socket", "key": "socket:10.66.0.20:443", "role": "object"}
        ]
      }
    }
  ]
}
JSON

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/target.json" > "$RESULTS/e2e-incident-merge.target-data_plane.json"

"$BIN/sysarmor-databatch-append" --manager "127.0.0.1:$GRPC_PORT" --token "$TOKEN" --input "$TMP/source.json" > "$RESULTS/e2e-incident-merge.source-data_plane.json"

wait_contains "target incident" '"id":"inc-00000000000000000001"' "$RESULTS/e2e-incident-merge.target.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list --scenario incident-merge-target
wait_contains "source incident" '"id":"inc-00000000000000000002"' "$RESULTS/e2e-incident-merge.source.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list --scenario incident-merge-source

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incident merge \
  --target-incident-id inc-00000000000000000001 \
  --source-incident-id inc-00000000000000000002 > "$RESULTS/e2e-incident-merge.merge.json"

for want in '"id":"inc-00000000000000000001"' '"lin-merge-target"' '"lin-merge-source"' '"id":"process:p-source"' '"id":"socket:10.66.0.20:443"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-incident-merge.merge.json"; then
    echo "[e2e-incident-merge][ERROR] merge response missing $want" >&2
    cat "$RESULTS/e2e-incident-merge.merge.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager incidents list > "$RESULTS/e2e-incident-merge.all.json"
if grep -Fq '"id":"inc-00000000000000000002"' "$RESULTS/e2e-incident-merge.all.json"; then
  echo "[e2e-incident-merge][ERROR] source incident still queryable after merge" >&2
  cat "$RESULTS/e2e-incident-merge.all.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-incident-merge.manager.log"
echo "[e2e-incident-merge] ok"
