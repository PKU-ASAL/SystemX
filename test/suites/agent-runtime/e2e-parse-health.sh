#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-parse-health.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((30000 + RANDOM % 5000))}"
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

echo "[e2e-agent-parse-health] building binaries"
make -C "$ROOT" build >/dev/null

mkdir -p "$TMP/bundle/bin"
cat > "$TMP/bundle/bin/tetragon" <<'SCRIPT'
#!/usr/bin/env sh
sleep 300
SCRIPT
chmod +x "$TMP/bundle/bin/tetragon"

cat > "$TMP/bundle/bin/tetra" <<'SCRIPT'
#!/usr/bin/env sh
if [ "$1" = "getevents" ]; then
  while true; do
    printf '%s\n' '{bad-json}'
    sleep 0.05
  done
fi
SCRIPT
chmod +x "$TMP/bundle/bin/tetra"

tetragon_sum="$(sha256sum "$TMP/bundle/bin/tetragon" | awk '{print $1}')"
tetra_sum="$(sha256sum "$TMP/bundle/bin/tetra" | awk '{print $1}')"
cat > "$TMP/bundle/manifest.json" <<EOF
{
  "version": "parse-health",
  "files": {
    "bin/tetragon": { "sha256": "$tetragon_sum" },
    "bin/tetra": { "sha256": "$tetra_sum" }
  }
}
EOF

cat > "$TMP/policy.yaml" <<'POLICY'
{"behaviors":["process.exec"],"observe_only":true}
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-parse-health
  host_id: e2e-host
  tenant_id: default
  token: $TOKEN

manager:
  address: 127.0.0.1:$GRPC_PORT
  transport: grpc

control:
  socket_path: $TMP/agent.sock

content:
  path: $TMP/content

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: $TMP/bundle
  install_dir: $TMP/install
  policy_path: $TMP/policy.yaml
  observe_only: true
  restart: always
  max_restarts: 1
  max_parse_errors: 1
  max_dropped_events: 0
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
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-parse-health][ERROR] timeout waiting for $needle via $cmd_name" >&2
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

wait_contains "agent-health degraded" '"status":"degraded"' "$RESULTS/e2e-agent-parse-health.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-parse-health --tenant-id default
wait_contains "agent-health parse counter" '"parse_errors":' "$RESULTS/e2e-agent-parse-health.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-parse-health --tenant-id default
wait_contains "agent-health last error" 'unrecognized tetragon event' "$RESULTS/e2e-agent-parse-health.health.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager health get --agent-id e2e-agent-parse-health --tenant-id default
wait_contains "tamper signal" 'sensor_tamper_or_blindness' "$RESULTS/e2e-agent-parse-health.signals.json" \
  "$BIN/sysarmorctl" --manager-url "$MGR_URL" --json manager signals list --label scenario=agent-health --layer endpoint --terminal true

cp "$TMP/agent.log" "$RESULTS/e2e-agent-parse-health.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-parse-health.manager.log"

echo "[e2e-agent-parse-health] ok"
