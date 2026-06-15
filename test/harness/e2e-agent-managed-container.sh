#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
RESULTS="$ROOT/.results"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
WORK="/tmp/sysarmor-agent-managed-container"

mkdir -p "$RESULTS"

cleanup() {
  docker exec mgr sh -c 'if [ -f /tmp/sysarmor-agent-managed-container/agent.pid ]; then kill "$(cat /tmp/sysarmor-agent-managed-container/agent.pid)" 2>/dev/null || true; fi' >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "[e2e-agent-managed-container] starting container topology"
bash "$HERE/start-container.sh" >/dev/null

echo "[e2e-agent-managed-container] preparing fake Tetragon bundle in mgr container"
docker exec mgr sh -c "rm -rf '$WORK'; mkdir -p '$WORK/bundle/bin' '$WORK/install' '$WORK/spool'"
docker exec mgr sh -c "cat > '$WORK/bundle/bin/tetragon' <<'EOF'
#!/usr/bin/env sh
COUNT=\"\${SYSARMOR_TETRAGON_COUNT:-/tmp/sysarmor-managed-tetragon-count}\"
n=0
if [ -f \"\$COUNT\" ]; then n=\"\$(cat \"\$COUNT\")\"; fi
n=\$((n + 1))
printf '%s' \"\$n\" > \"\$COUNT\"
sleep 300
EOF
chmod +x '$WORK/bundle/bin/tetragon'
cat > '$WORK/bundle/bin/tetra' <<'EOF'
#!/usr/bin/env sh
if [ \"\$1\" = \"getevents\" ]; then
  printf '%s\n' '{\"process_exec\":{\"process\":{\"pid\":200,\"uid\":0,\"binary\":\"/bin/bash\",\"arguments\":\"-c id\",\"start_time\":\"2026-06-14T10:00:00Z\"},\"parent\":{\"pid\":1,\"binary\":\"/sbin/init\",\"start_time\":\"2026-06-14T09:59:59Z\"}},\"node_name\":\"container-host\",\"time\":\"2026-06-14T10:00:00Z\"}'
  sleep 300
fi
EOF
chmod +x '$WORK/bundle/bin/tetra'"

docker exec mgr sh -c "tetragon_sum=\$(sha256sum '$WORK/bundle/bin/tetragon' | awk '{print \$1}'); tetra_sum=\$(sha256sum '$WORK/bundle/bin/tetra' | awk '{print \$1}'); cat > '$WORK/bundle/manifest.json' <<EOF
{
  \"version\": \"container-managed\",
  \"files\": {
    \"bin/tetragon\": { \"sha256\": \"\$tetragon_sum\" },
    \"bin/tetra\": { \"sha256\": \"\$tetra_sum\" }
  }
}
EOF"

docker exec mgr sh -c "cat > '$WORK/policy.yaml' <<'EOF'
kinds: [EXEC]
EOF
cat > '$WORK/agent.yaml' <<EOF
agent:
  id: container-agent-managed
  host_id: container-host
  tenant_id: default
  token: $TOKEN

manager:
  address: http://127.0.0.1:9443
  transport: http

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: $WORK/bundle
  install_dir: $WORK/install
  policy_path: $WORK/policy.yaml
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

spool:
  path: $WORK/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 100ms

upload:
  retry_initial: 50ms
  retry_max: 100ms
  request_timeout: 2s

health:
  interval: 100ms
EOF"

docker exec mgr curl -sf -X POST "http://127.0.0.1:9443/api/v1/reset" >/dev/null
docker exec mgr sh -c "rm -f '$WORK/agent.log' '$WORK/tetragon.count'; SYSARMOR_TETRAGON_COUNT='$WORK/tetragon.count' /opt/sysarmor/bin/sysarmor-agent run --config '$WORK/agent.yaml' > '$WORK/agent.log' 2>&1 & echo \$! > '$WORK/agent.pid'"

wait_contains() {
  local cmd_name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 30))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-managed-container][ERROR] timeout waiting for $needle via $cmd_name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- agent log ---" >&2
      docker exec mgr cat "$WORK/agent.log" >&2 2>/dev/null || true
      echo "--- bundle install ---" >&2
      docker exec mgr sh -c "find '$WORK/install' -maxdepth 4 -type f -o -type l | sort" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 0.2
  done
}

wait_contains "agent-health" '"backend":"tetragon"' "$RESULTS/e2e-agent-managed-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-agent-managed --tenant-id default
wait_contains "agent-health installed" '"installed":true' "$RESULTS/e2e-agent-managed-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-agent-managed --tenant-id default
wait_contains "agent-health policy" '"policy_loaded":true' "$RESULTS/e2e-agent-managed-container.health.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id container-agent-managed --tenant-id default
wait_contains "metrics" '"events_ingested":1' "$RESULTS/e2e-agent-managed-container.metrics.json" \
  docker exec mgr /opt/sysarmor/bin/sysarmorctl --mgr 127.0.0.1:9443 --json metrics

if [[ "$(docker exec mgr cat "$WORK/tetragon.count" 2>/dev/null | tr -d '\r')" != "1" ]]; then
  echo "[e2e-agent-managed-container][ERROR] tetragon process did not start exactly once" >&2
  docker exec mgr cat "$WORK/tetragon.count" >&2 2>/dev/null || true
  exit 1
fi

docker exec mgr test -x "$WORK/install/tetragon/current/bin/tetragon"
docker exec mgr test -x "$WORK/install/tetragon/current/bin/tetra"
docker exec mgr cat "$WORK/agent.log" > "$RESULTS/e2e-agent-managed-container.agent.log"

echo "[e2e-agent-managed-container] ok"
