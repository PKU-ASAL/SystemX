#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-outage-soak.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((32000 + RANDOM % 5000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"
EVENTS="${SYSARMOR_OUTAGE_EVENTS:-5}"

mkdir -p "$RESULTS" "$BIN" "$TMP/spool"

cleanup() {
  if [[ -n "${AGENT_PID:-}" ]]; then kill "$AGENT_PID" 2>/dev/null || true; fi
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-outage-soak] building binaries"
make -C "$ROOT" build >/dev/null

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-outage-soak
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
  fake_startup_events: $EVENTS
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

wait_for_spool_batches() {
  local want="$1"
  local deadline=$((SECONDS + 10))
  local count=0
  until [[ "$count" -ge "$want" ]]; do
    count="$(find "$TMP/spool" -maxdepth 1 -name '*.batch.json' 2>/dev/null | wc -l || true)"
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-outage-soak][ERROR] timeout waiting for $want local spool batches, got $count" >&2
      ls -la "$TMP/spool" >&2 2>/dev/null || true
      echo "--- agent log ---" >&2
      cat "$TMP/agent.log" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.1
  done
}

wait_contains() {
  local name="$1"
  local url="$2"
  local needle="$3"
  local out="$4"
  local deadline=$((SECONDS + 15))
  until curl -sf "$url" >"$out" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-outage-soak][ERROR] timeout waiting for $needle via $name at $url" >&2
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

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

wait_for_spool_batches "$EVENTS"
mkdir -p "$RESULTS/e2e-agent-outage-soak.before"
cp "$TMP"/spool/*.batch.json "$RESULTS/e2e-agent-outage-soak.before/"

"$BIN/sysarmor-manager" \
  --listen "127.0.0.1:$MANAGER_PORT" \
  --grpc-listen "127.0.0.1:$GRPC_PORT" \
  --store "$TMP/store.json" \
  --dev-token "$TOKEN" \
  >"$TMP/manager.log" 2>&1 &
MGR_PID=$!

wait_contains "manager healthz" "$MGR_URL/healthz" '"ok":true' "$TMP/healthz.json"
wait_contains "metrics" "$MGR_URL/api/v1/metrics" "\"events_ingested\":$EVENTS" "$RESULTS/e2e-agent-outage-soak.metrics.json"
wait_contains "agent-health" "$MGR_URL/api/v1/agent-health?agent_id=e2e-agent-outage-soak&tenant_id=default" '"queued_batches":0' "$RESULTS/e2e-agent-outage-soak.health.json"
wait_contains "agent-health upload" "$MGR_URL/api/v1/agent-health?agent_id=e2e-agent-outage-soak&tenant_id=default" '"remaining_batches":0' "$RESULTS/e2e-agent-outage-soak.health.json"

deadline=$((SECONDS + 15))
while compgen -G "$TMP/spool/*.batch.json" >/dev/null; do
  if (( SECONDS >= deadline )); then
    echo "[e2e-agent-outage-soak][ERROR] spool did not fully drain" >&2
    ls -la "$TMP/spool" >&2
    exit 1
  fi
  sleep 0.1
done

curl -sf "$MGR_URL/api/v1/events?scenario=" > "$RESULTS/e2e-agent-outage-soak.events.json"
python3 - "$RESULTS/e2e-agent-outage-soak.events.json" "$EVENTS" <<'PY'
import json
import sys

events = json.loads(open(sys.argv[1]).read())
want = int(sys.argv[2])
matching = [ev for ev in events if ev.get("agent_id") == "e2e-agent-outage-soak"]
if len(matching) != want:
    raise SystemExit(f"uploaded event count={len(matching)}, want {want}: {matching}")
ids = [ev.get("id") for ev in matching]
if len(set(ids)) != want:
    raise SystemExit(f"uploaded event ids are not unique: {ids}")
PY

if ! grep -Fq 'connect: connection refused' "$TMP/agent.log"; then
  echo "[e2e-agent-outage-soak][ERROR] expected outage evidence in agent log" >&2
  cat "$TMP/agent.log" >&2
  exit 1
fi

cp "$TMP/agent.log" "$RESULTS/e2e-agent-outage-soak.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-outage-soak.manager.log"

echo "[e2e-agent-outage-soak] ok"
