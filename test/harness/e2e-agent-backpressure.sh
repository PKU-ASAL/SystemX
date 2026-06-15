#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-backpressure.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((31000 + RANDOM % 4000))}"
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

echo "[e2e-agent-backpressure] building binaries"
make -C "$ROOT" build >/dev/null

cat > "$TMP/policy.yaml" <<'POLICY'
kinds: [EXEC]
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-backpressure
  host_id: e2e-host
  tenant_id: default
  token: $TOKEN

manager:
  address: $MGR_URL
  transport: http

sensor:
  backend: fake
  mode: managed
  policy_path: $TMP/policy.yaml
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

spool:
  path: $TMP/spool
  max_bytes: 1
  batch_size: 256
  flush_interval: 100ms

upload:
  retry_initial: 50ms
  retry_max: 100ms
  request_timeout: 2s

health:
  interval: 100ms
EOF

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store "$TMP/store.json" \
  --dev-token "$TOKEN" \
  >"$TMP/manager.log" 2>&1 &
MGR_PID=$!

wait_contains() {
  local cmd_name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 10))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-backpressure][ERROR] timeout waiting for $needle via $cmd_name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
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

wait_contains "agent-health degraded" '"status":"degraded"' "$RESULTS/e2e-agent-backpressure.health.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-health --agent-id e2e-agent-backpressure --tenant-id default
wait_contains "agent-health backpressure count" '"backpressure_count":1' "$RESULTS/e2e-agent-backpressure.health.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-health --agent-id e2e-agent-backpressure --tenant-id default
wait_contains "agent-health dropped batches" '"dropped_batches":1' "$RESULTS/e2e-agent-backpressure.health.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-health --agent-id e2e-agent-backpressure --tenant-id default
wait_contains "agent-health last error" 'spool max_bytes exceeded' "$RESULTS/e2e-agent-backpressure.health.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-health --agent-id e2e-agent-backpressure --tenant-id default

if compgen -G "$TMP/spool/*.batch.json" >/dev/null; then
  echo "[e2e-agent-backpressure][ERROR] expected no persisted batches after forced drop" >&2
  ls -la "$TMP/spool" >&2
  exit 1
fi

cp "$TMP/agent.log" "$RESULTS/e2e-agent-backpressure.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-backpressure.manager.log"

echo "[e2e-agent-backpressure] ok"
