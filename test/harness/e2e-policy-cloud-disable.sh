#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-policy-cloud-disable.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((36000 + RANDOM % 2000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
SCENARIO="apt-staged-drop-policy"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-policy-cloud-disable] building binaries"
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
      echo "[e2e-policy-cloud-disable][ERROR] $name missing $needle" >&2
      cat "$out" >&2 2>/dev/null || true
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "healthz" '"ok":true' "$TMP/health.json" curl -sf "$MGR_URL/healthz"

cat > "$TMP/no-cross-policy.json" <<'JSON'
{
  "policy_id": "no-cross-incident",
  "version": 1,
  "tenant_id": "default",
  "endpoint_rules": [
    "web_runtime_spawns_shell",
    "download_by_lolbin",
    "payload_dropped",
    "reverse_shell_pattern",
    "suspicious_exec_connect"
  ],
  "cloud_rules": ["web_shell_chain"],
  "mode": "observe",
  "converge": {
    "mode": "rarity_structural",
    "top_k": 8,
    "max_path_hops": 6,
    "cross_lineage": true
  },
  "published": true
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/policies" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/no-cross-policy.json" > "$RESULTS/e2e-policy-cloud-disable.policy.json"

cat > "$TMP/assignment.json" <<'JSON'
{
  "tenant_id": "default",
  "agent_id": "policy-agent",
  "policy_id": "no-cross-incident"
}
JSON

curl -sf -X POST "$MGR_URL/api/v1/policy-assignments" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/assignment.json" > "$RESULTS/e2e-policy-cloud-disable.assignment.json"

wait_contains "effective policy" '"policy_id":"no-cross-incident"' "$RESULTS/e2e-policy-cloud-disable.effective.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json effective-policy --tenant-id default --agent-id policy-agent

cat > "$TMP/batch.json" <<EOF
{
  "agent": {
    "agent_id": "policy-agent",
    "host_id": "policy-host",
    "tenant_id": "default",
    "version": "e2e"
  },
  "signals": [
    {
      "name": "payload_dropped",
      "where": "SIGNAL_WHERE_ENDPOINT",
      "base_risk": 50,
      "global_rarity": 1,
      "lineage_id": "lin-drop",
      "scenario": "$SCENARIO",
      "entities": [{"kind": "file", "key": "file:/var/lib/app/plugins/helper", "role": "object"}]
    },
    {
      "name": "suspicious_exec_connect",
      "where": "SIGNAL_WHERE_ENDPOINT",
      "base_risk": 50,
      "global_rarity": 1,
      "lineage_id": "lin-connect",
      "scenario": "$SCENARIO",
      "entities": [
        {"kind": "file", "key": "file:/var/lib/app/plugins/helper", "role": "object"},
        {"kind": "socket", "key": "socket:10.66.0.99:443", "role": "object"}
      ]
    }
  ],
  "batch_id": "policy-cloud-disable-1"
}
EOF

curl -sf -X POST "$MGR_URL/api/v1/upload" \
  -H "X-SysArmor-Agent-Token: $TOKEN" \
  -H 'Content-Type: application/json' \
  --data-binary @"$TMP/batch.json" > "$RESULTS/e2e-policy-cloud-disable.ack.json"

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json signals --scenario "$SCENARIO" --layer cloud > "$RESULTS/e2e-policy-cloud-disable.cloud-signals.json"
if grep -Fq 'dropped_payload_executed_and_connects' "$RESULTS/e2e-policy-cloud-disable.cloud-signals.json"; then
  echo "[e2e-policy-cloud-disable][ERROR] disabled cloud rule still emitted signal" >&2
  cat "$RESULTS/e2e-policy-cloud-disable.cloud-signals.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json incidents --scenario "$SCENARIO" > "$RESULTS/e2e-policy-cloud-disable.incidents.json"
if grep -Fq '"inc-' "$RESULTS/e2e-policy-cloud-disable.incidents.json"; then
  echo "[e2e-policy-cloud-disable][ERROR] disabled cloud rule still created incident" >&2
  cat "$RESULTS/e2e-policy-cloud-disable.incidents.json" >&2
  exit 1
fi

"$BIN/sysarmorctl" --mgr "$MGR_URL" --json policy-assignments --tenant-id default --agent-id policy-agent > "$RESULTS/e2e-policy-cloud-disable.assignments.json"
if ! grep -Fq '"policy_id":"no-cross-incident"' "$RESULTS/e2e-policy-cloud-disable.assignments.json"; then
  echo "[e2e-policy-cloud-disable][ERROR] assignment not queryable" >&2
  cat "$RESULTS/e2e-policy-cloud-disable.assignments.json" >&2
  exit 1
fi

cp "$TMP/manager.log" "$RESULTS/e2e-policy-cloud-disable.manager.log"
echo "[e2e-policy-cloud-disable] ok"
