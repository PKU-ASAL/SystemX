#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-restart.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((22000 + RANDOM % 20000))}"
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

echo "[e2e-agent-sensor-restart] building binaries"
make -C "$ROOT" build >/dev/null

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

mkdir -p "$TMP/bundle/bin" "$TMP/install"
cat > "$TMP/bundle/bin/tetragon" <<'SCRIPT'
#!/usr/bin/env sh
COUNT="${SYSARMOR_TETRAGON_COUNT:-/tmp/sysarmor-tetragon-count}"
n=0
if [ -f "$COUNT" ]; then n="$(cat "$COUNT")"; fi
n=$((n + 1))
printf '%s' "$n" > "$COUNT"
exit 7
SCRIPT
chmod +x "$TMP/bundle/bin/tetragon"

cat > "$TMP/bundle/bin/tetra" <<'SCRIPT'
#!/usr/bin/env sh
if [ "$1" = "getevents" ]; then
  printf '%s\n' '{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"e2e-host","time":"2026-06-14T10:00:00Z"}'
  sleep 20
fi
SCRIPT
chmod +x "$TMP/bundle/bin/tetra"

tetragon_sum="$(sha256sum "$TMP/bundle/bin/tetragon" | awk '{print $1}')"
tetra_sum="$(sha256sum "$TMP/bundle/bin/tetra" | awk '{print $1}')"
cat > "$TMP/bundle/manifest.json" <<EOF
{
  "version": "e2e-restart",
  "files": {
    "bin/tetragon": { "sha256": "$tetragon_sum" },
    "bin/tetra": { "sha256": "$tetra_sum" }
  }
}
EOF

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-sensor-restart
  host_id: e2e-host
  tenant_id: default
  token: $TOKEN

manager:
  address: 127.0.0.1:$GRPC_PORT
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: $TMP/bundle
  install_dir: $TMP/install
  policy_path: $TMP/policy.yaml
  observe_only: true
  restart: always
  max_restarts: 2
  restart_window: 50ms

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
  --store-backend memory \
  --dev-token "$TOKEN" \
  >"$TMP/manager.log" 2>&1 &
MGR_PID=$!

wait_contains() {
  local url="$1"
  local needle="$2"
  local out="$3"
  local deadline=$((SECONDS + 10))
  until curl -sf "$url" >"$out" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-sensor-restart][ERROR] timeout waiting for $needle at $url" >&2
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

wait_contains "http://127.0.0.1:$MANAGER_PORT/healthz" '"ok":true' "$TMP/healthz.json"

SYSARMOR_TETRAGON_COUNT="$TMP/tetragon.count" "$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1 &
AGENT_PID=$!

wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/agent-health?agent_id=e2e-sensor-restart&tenant_id=default" '"status":"degraded"' "$RESULTS/e2e-agent-sensor-restart.health.json"
wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/agent-health?agent_id=e2e-sensor-restart&tenant_id=default" '"restart_count":3' "$RESULTS/e2e-agent-sensor-restart.health.json"
wait_contains "http://127.0.0.1:$MANAGER_PORT/api/v1/signals?scenario=agent-health&layer=endpoint&terminal=true" 'sensor_tamper_or_blindness' "$RESULTS/e2e-agent-sensor-restart.signals.json"

if [[ "$(cat "$TMP/tetragon.count")" != "2" ]]; then
  echo "[e2e-agent-sensor-restart][ERROR] tetragon restart count file=$(cat "$TMP/tetragon.count" 2>/dev/null || echo missing)" >&2
  exit 1
fi

cp "$TMP/agent.log" "$RESULTS/e2e-agent-sensor-restart.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-sensor-restart.manager.log"

echo "[e2e-agent-sensor-restart] ok"
