#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-shutdown.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((26000 + RANDOM % 10000))}"
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

echo "[e2e-agent-shutdown] building binaries"
make -C "$ROOT" build >/dev/null

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-shutdown
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
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

spool:
  path: $TMP/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 1h

upload:
  retry_initial: 50ms
  retry_max: 100ms
  request_timeout: 2s

health:
  interval: 1h
EOF

wait_for_spool_batch() {
  local deadline=$((SECONDS + 10))
  until compgen -G "$TMP/spool/*.batch.json" >/dev/null; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-shutdown][ERROR] timeout waiting for local spool batch" >&2
      echo "--- agent log ---" >&2
      cat "$TMP/agent.log" >&2 2>/dev/null || true
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
      echo "[e2e-agent-shutdown][ERROR] timeout waiting for $needle at $url" >&2
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

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store-backend memory \
  --dev-token "$TOKEN" \
  >"$TMP/manager.log" 2>&1 &
MGR_PID=$!

wait_contains "http://127.0.0.1:$MANAGER_PORT/healthz" '"ok":true' "$TMP/healthz.json"

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

wait_for_spool_batch
cp "$TMP"/spool/*.batch.json "$RESULTS/e2e-agent-shutdown.before.batch.json"

kill -TERM "$AGENT_PID"
wait "$AGENT_PID" || true
AGENT_PID=""

wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/metrics" '"events_ingested":1' "$RESULTS/e2e-agent-shutdown.metrics.json"
wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/events?scenario=" '"agent_id":"e2e-agent-shutdown"' "$RESULTS/e2e-agent-shutdown.events.json"
wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/agent-health?agent_id=e2e-agent-shutdown&tenant_id=default" '"status":"degraded"' "$RESULTS/e2e-agent-shutdown.health.final.json"
wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/agent-health?agent_id=e2e-agent-shutdown&tenant_id=default" '"running":false' "$RESULTS/e2e-agent-shutdown.health.final.json"
wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/agent-health?agent_id=e2e-agent-shutdown&tenant_id=default" '"queued_batches":0' "$RESULTS/e2e-agent-shutdown.health.final.json"
wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/agent-health?agent_id=e2e-agent-shutdown&tenant_id=default" '"remaining_batches":0' "$RESULTS/e2e-agent-shutdown.health.final.json"

deadline=$((SECONDS + 10))
while compgen -G "$TMP/spool/*.batch.json" >/dev/null; do
  if (( SECONDS >= deadline )); then
    echo "[e2e-agent-shutdown][ERROR] spool did not drain after SIGTERM" >&2
    ls -la "$TMP/spool" >&2
    echo "--- agent log ---" >&2
    cat "$TMP/agent.log" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 0.1
done

if ! grep -Fq 'agent shutdown drain: uploaded=1 remaining=0' "$TMP/agent.log"; then
  echo "[e2e-agent-shutdown][ERROR] expected shutdown drain evidence in agent log" >&2
  cat "$TMP/agent.log" >&2
  exit 1
fi
if ! grep -Fq 'agent final health: sensor=fake running=false policy_loaded=true status=degraded queued_batches=0' "$TMP/agent.log"; then
  echo "[e2e-agent-shutdown][ERROR] expected final health to report drained queue" >&2
  cat "$TMP/agent.log" >&2
  exit 1
fi

cp "$TMP/agent.log" "$RESULTS/e2e-agent-shutdown.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-shutdown.manager.log"

echo "[e2e-agent-shutdown] ok"
