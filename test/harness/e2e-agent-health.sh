#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-health.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((26000 + RANDOM % 20000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]]; then kill "$AGENT_PID" 2>/dev/null || true; fi
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-health] building binaries"
make -C "$ROOT" build >/dev/null

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-health
  host_id: e2e-host
  tenant_id: default
  token: $TOKEN

manager:
  address: 127.0.0.1:$GRPC_PORT
  transport: grpc

sensor:
  backend: fake
  mode: managed
  policy_path: $TMP/policy.yaml
  scope:
    type: container
    selector: e2e-scope
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

spool:
  path: $TMP/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 100ms

data_plane:
  retry_initial: 50ms
  retry_max: 100ms
  request_timeout: 2s

health:
  interval: 100ms
EOF

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store-backend memory \
  --dev-token "$TOKEN" \
  >"$TMP/manager.log" 2>&1 &
MGR_PID=$!

wait_contains() {
  local cmd_name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 10))
  until "$@" >"$out" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-health][ERROR] timeout waiting for $needle via $cmd_name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- agent log ---" >&2
      cat "$TMP/agent.log" >&2 2>/dev/null || true
      echo "--- manager log ---" >&2
      cat "$TMP/manager.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.1
  done
}

wait_contains "manager healthz" '"ok":true' "$TMP/healthz.json" curl -sf "$MGR_URL/healthz"

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

wait_contains "sysarmorctl agents" "e2e-agent-health" "$RESULTS/e2e-agent-health.agents.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager agents list
wait_contains "sysarmorctl agents tenant" '"tenant_id":"default"' "$RESULTS/e2e-agent-health.agents.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager agents list
wait_contains "sysarmorctl agents health status" '"health_status":"ok"' "$RESULTS/e2e-agent-health.agents.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager agents list
wait_contains "sysarmorctl agents scope" '"scope":{"type":"container","selector":"e2e-scope"}' "$RESULTS/e2e-agent-health.agents.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager agents list
wait_contains "sysarmorctl agents filtered" '"agent_id":"e2e-agent-health"' "$RESULTS/e2e-agent-health.agents-filtered.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager agents list --tenant-id default --scope-type container --scope-selector e2e-scope --health-status ok
wait_contains "sysarmorctl agents filtered scope" '"scope":{"type":"container","selector":"e2e-scope"}' "$RESULTS/e2e-agent-health.agents-filtered.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager agents list --tenant-id default --scope-type container --scope-selector e2e-scope --health-status ok
wait_contains "sysarmorctl agents filtered capability" '"sensor_capability"' "$RESULTS/e2e-agent-health.agents-filtered.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager agents list --tenant-id default --scope-type container --scope-selector e2e-scope --health-status ok
wait_contains "sysarmorctl agent-health" '"agent_id":"e2e-agent-health"' "$RESULTS/e2e-agent-health.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-health --tenant-id default
wait_contains "sysarmorctl agent-health sensor" '"sensor_health"' "$RESULTS/e2e-agent-health.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-health --tenant-id default
wait_contains "sysarmorctl agent-health scope type" '"scope":{"type":"container","selector":"e2e-scope"}' "$RESULTS/e2e-agent-health.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-health --tenant-id default
wait_contains "sysarmorctl metrics" '"events_ingested":1' "$RESULTS/e2e-agent-health.metrics.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager metrics

cp "$TMP/agent.log" "$RESULTS/e2e-agent-health.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-health.manager.log"

echo "[e2e-agent-health] ok"
