#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-incident-lifecycle.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((52000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
SCENARIO="incident-lifecycle"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-incident-lifecycle] building binaries"
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
      echo "[e2e-incident-lifecycle][ERROR] $name missing $needle" >&2
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
  "batch_id": "incident-lifecycle-batch",
  "agent": {
    "agent_id": "incident-lifecycle-agent",
    "host_id": "incident-lifecycle-host",
    "tenant_id": "default",
    "version": "e2e"
  },
  "signals": [
    {
      "id": "sig-lifecycle-web",
      "name": "web_runtime_spawns_shell",
      "where": "SIGNAL_WHERE_ENDPOINT",
      "base_risk": 50,
      "global_rarity": 1,
      "lineage_id": "lin-life",
      "scenario": "$SCENARIO",
      "entities": [{"kind": "process", "key": "process:p-web", "role": "subject"}]
    },
    {
      "id": "sig-lifecycle-rev",
      "name": "reverse_shell_pattern",
      "where": "SIGNAL_WHERE_ENDPOINT",
      "base_risk": 80,
      "global_rarity": 1,
      "lineage_id": "lin-life",
      "terminal": true,
      "scenario": "$SCENARIO",
      "entities": [
        {"kind": "process", "key": "process:p-bash", "role": "subject"},
        {"kind": "socket", "key": "socket:10.66.0.99:443", "role": "object"}
      ]
    }
  ]
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/upload" \
  -H "X-SysArmor-Agent-Token: $TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/batch.json" > "$RESULTS/e2e-incident-lifecycle.upload.json"

wait_contains "incident open" '"status":"open"' "$RESULTS/e2e-incident-lifecycle.open.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json incidents --scenario "$SCENARIO"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json incident-lifecycle \
  --scenario "$SCENARIO" \
  --status suppressed \
  --reason "known drill" \
  --actor e2e > "$RESULTS/e2e-incident-lifecycle.suppress.json"

for want in '"status":"suppressed"' '"status_reason":"known drill"' '"status_actor":"e2e"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-incident-lifecycle.suppress.json"; then
    echo "[e2e-incident-lifecycle][ERROR] suppress missing $want" >&2
    cat "$RESULTS/e2e-incident-lifecycle.suppress.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json incident-lifecycle \
  --scenario "$SCENARIO" \
  --status closed \
  --reason "triaged" \
  --actor e2e > "$RESULTS/e2e-incident-lifecycle.close.json"

for want in '"status":"closed"' '"status_reason":"triaged"'; do
  if ! grep -Fq "$want" "$RESULTS/e2e-incident-lifecycle.close.json"; then
    echo "[e2e-incident-lifecycle][ERROR] close missing $want" >&2
    cat "$RESULTS/e2e-incident-lifecycle.close.json" >&2
    exit 1
  fi
done

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json incident-lifecycle \
  --scenario "$SCENARIO" \
  --status open \
  --reason "reopened" \
  --actor e2e > "$RESULTS/e2e-incident-lifecycle.reopen.json"

if ! grep -Fq '"status":"open"' "$RESULTS/e2e-incident-lifecycle.reopen.json"; then
  echo "[e2e-incident-lifecycle][ERROR] reopen did not restore open status" >&2
  cat "$RESULTS/e2e-incident-lifecycle.reopen.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-incident-lifecycle.manager.log"
echo "[e2e-incident-lifecycle] ok"
