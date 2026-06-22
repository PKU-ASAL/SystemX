#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RESULTS="$ROOT/test/.results"
BIN="$ROOT/bin"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-capability-btf.XXXXXX")"
PORT_BASE="${PORT_BASE:-$((32000 + RANDOM % 3000))}"
MANAGER_PORT="${MANAGER_PORT:-$PORT_BASE}"
GRPC_PORT="${GRPC_PORT:-$((PORT_BASE + 1))}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
MGR_URL="http://127.0.0.1:$MANAGER_PORT"

mkdir -p "$RESULTS" "$BIN"

cleanup() {
  if [[ -n "${MGR_PID:-}" ]]; then kill "$MGR_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

echo "[e2e-agent-capability-btf] building binaries"
make -C "$ROOT" build >/dev/null

mkdir -p "$TMP/bundle/bin" "$TMP/install"
printf '%s\n' '#!/bin/sh' 'exit 0' > "$TMP/bundle/bin/tetragon"
printf '%s\n' '#!/bin/sh' 'exit 0' > "$TMP/bundle/bin/tetra"
chmod +x "$TMP/bundle/bin/tetragon" "$TMP/bundle/bin/tetra"
TETRAGON_SHA="$(sha256sum "$TMP/bundle/bin/tetragon" | awk '{print $1}')"
TETRA_SHA="$(sha256sum "$TMP/bundle/bin/tetra" | awk '{print $1}')"

cat > "$TMP/bundle/manifest.json" <<EOF
{
  "version": "capability-btf",
  "files": {
    "bin/tetragon": { "sha256": "$TETRAGON_SHA" },
    "bin/tetra": { "sha256": "$TETRA_SHA" }
  }
}
EOF

cat > "$TMP/policy.yaml" <<'EOF'
{"behaviors":["process.exec"],"observe_only":true}
EOF

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: e2e-agent-capability-btf
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
  btf_path: $TMP/missing-vmlinux
  require_btf: true
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
      echo "[e2e-agent-capability-btf][ERROR] timeout waiting for $needle via $cmd_name" >&2
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

set +e
"$BIN/sysarmor-agent" run --config "$TMP/agent.yaml" >"$TMP/agent.log" 2>&1
AGENT_RC=$?
set -e
if [[ "$AGENT_RC" -eq 0 ]]; then
  echo "[e2e-agent-capability-btf][ERROR] agent unexpectedly succeeded with missing required BTF" >&2
  cat "$TMP/agent.log" >&2
  exit 1
fi

wait_contains "agent-health degraded" '"status":"degraded"' "$RESULTS/e2e-agent-capability-btf.health.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-health --agent-id e2e-agent-capability-btf --tenant-id default
wait_contains "agent-health stopped" '"running":false' "$RESULTS/e2e-agent-capability-btf.health.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-health --agent-id e2e-agent-capability-btf --tenant-id default
wait_contains "agent-health btf error" 'btf unavailable' "$RESULTS/e2e-agent-capability-btf.health.json" \
  "$BIN/sysarmorctl" --mgr "$MGR_URL" --json agent-health --agent-id e2e-agent-capability-btf --tenant-id default

cp "$TMP/agent.log" "$RESULTS/e2e-agent-capability-btf.agent.log"
cp "$TMP/manager.log" "$RESULTS/e2e-agent-capability-btf.manager.log"

echo "[e2e-agent-capability-btf] ok"
