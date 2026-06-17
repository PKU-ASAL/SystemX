#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-daemon.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((20000 + RANDOM % 20000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]]; then kill "$AGENT_PID" 2>/dev/null || true; fi
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-daemon] building binaries"
make -C "$ROOT" build >/dev/null

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-daemon
  host_id: e2e-host
  tenant_id: default
  token: $TOKEN

manager:
  address: http://127.0.0.1:$MANAGER_PORT
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
  max_bytes: 268435456
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

wait_http() {
  local url="$1"
  local deadline=$((SECONDS + 10))
  until curl -sf "$url" >/dev/null; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-daemon][ERROR] timeout waiting for $url" >&2
      cat "$TMP/manager.log" >&2 || true
      exit 1
    fi
    sleep 0.1
  done
}

wait_contains() {
  local url="$1"
  local needle="$2"
  local out="$3"
  local deadline=$((SECONDS + 10))
  until curl -sf "$url" >"$out" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-daemon][ERROR] timeout waiting for $needle at $url" >&2
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

wait_http "http://127.0.0.1:$MANAGER_PORT/healthz"

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/agent-health?agent_id=e2e-agent-daemon&tenant_id=default" '"agent_id":"e2e-agent-daemon"' "$RESULTS/e2e-agent-daemon.health.json"
wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/metrics" '"events_ingested":1' "$RESULTS/e2e-agent-daemon.metrics.json"

cp "$TMP/agent.log" "$RESULTS/e2e-agent-daemon.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-daemon.manager.log"

echo "[e2e-agent-daemon] ok"
