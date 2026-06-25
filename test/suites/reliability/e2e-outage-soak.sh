#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../../harness/lib/common.sh"

sa_init_repo_paths
TMP="$(sa_make_tmp sysarmor-agent-outage-soak)"
sa_pick_ports 32000 5000
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
EVENTS="${SYSARMOR_OUTAGE_EVENTS:-5}"
SA_TEST_NAME="e2e-agent-outage-soak"
SA_WAIT_LOGS=("$TMP/agent.log" "$TMP/manager.log")

mkdir -p "$TMP/spool"

cleanup() {
  sa_kill_pid_ref AGENT_PID
  sa_kill_pid_ref MGR_PID
  sa_cleanup_tmp "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-outage-soak] building binaries"
sa_build_all

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
  address: 127.0.0.1:$GRPC_PORT
  transport: grpc

control:
  socket_path: $TMP/agent.sock

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

content:
  path: $TMP/content

data_plane:
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

"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

wait_for_spool_batches "$EVENTS"
mkdir -p "$RESULTS/e2e-agent-outage-soak.before"
cp "$TMP"/spool/*.batch.json "$RESULTS/e2e-agent-outage-soak.before/"

sa_start_memory_manager --dev-token "$TOKEN"

SA_WAIT_TIMEOUT=15 sa_wait_url_contains "$MGR_URL/healthz" '"ok":true' "$TMP/healthz.json"
SA_WAIT_TIMEOUT=15 sa_wait_url_contains "$MGR_URL/api/v1/metrics" "\"events_ingested\":$EVENTS" "$RESULTS/e2e-agent-outage-soak.metrics.json"
SA_WAIT_TIMEOUT=15 sa_wait_url_contains "$MGR_URL/api/v1/agent-health?agent_id=e2e-agent-outage-soak&tenant_id=default" '"queued_batches":0' "$RESULTS/e2e-agent-outage-soak.health.json"
SA_WAIT_TIMEOUT=15 sa_wait_url_contains "$MGR_URL/api/v1/agent-health?agent_id=e2e-agent-outage-soak&tenant_id=default" '"remaining_batches":0' "$RESULTS/e2e-agent-outage-soak.health.json"

SA_WAIT_TIMEOUT=15 sa_wait_no_glob "$TMP/spool/*.batch.json" "spool drain"

curl -sf "$MGR_URL/api/v1/events?" > "$RESULTS/e2e-agent-outage-soak.events.json"
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
