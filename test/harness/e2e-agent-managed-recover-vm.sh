#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
ENVDIR="$(cd "$ROOT/env/vm" && pwd)"
RESULTS="$ROOT/.results"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-managed-recover-vm.XXXXXX")"

mkdir -p "$RESULTS"

cleanup() {
  rm -rf "$TMP"
  (
    cd "$ENVDIR"
    vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true" >/dev/null 2>&1 || true
  )
}
trap cleanup EXIT

echo "[e2e-agent-managed-recover-vm] building recovering fake Tetragon bundle"
mkdir -p "$TMP/bundle/bin"
cat > "$TMP/bundle/bin/tetragon" <<'SCRIPT'
#!/usr/bin/env sh
COUNT="${SYSARMOR_TETRAGON_COUNT:-/tmp/sysarmor-managed-recover-vm-tetragon-count}"
n=0
if [ -f "$COUNT" ]; then n="$(cat "$COUNT")"; fi
n=$((n + 1))
printf '%s' "$n" > "$COUNT"
if [ "$n" -eq 1 ]; then
  sleep 0.2
  exit 7
fi
sleep 300
SCRIPT
chmod +x "$TMP/bundle/bin/tetragon"

cat > "$TMP/bundle/bin/tetra" <<'SCRIPT'
#!/usr/bin/env sh
if [ "$1" = "getevents" ]; then
  while true; do
    printf '%s\n' '{"process_exec":{"process":{"pid":310,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":1,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"vm-node-a","time":"2026-06-14T10:00:00Z"}'
    sleep 0.1
  done
fi
SCRIPT
chmod +x "$TMP/bundle/bin/tetra"

tetragon_sum="$(sha256sum "$TMP/bundle/bin/tetragon" | awk '{print $1}')"
tetra_sum="$(sha256sum "$TMP/bundle/bin/tetra" | awk '{print $1}')"
cat > "$TMP/bundle/manifest.json" <<EOF
{
  "version": "vm-managed-recover",
  "files": {
    "bin/tetragon": { "sha256": "$tetragon_sum" },
    "bin/tetra": { "sha256": "$tetra_sum" }
  }
}
EOF

cat > "$TMP/policy.yaml" <<'POLICY'
kinds: [EXEC]
POLICY

cat > "$TMP/agent.yaml" <<EOF
agent:
  id: vm-agent-managed-recover
  host_id: vm-node-a
  tenant_id: default
  token: $TOKEN

manager:
  address: http://10.66.0.10:9443
  transport: http

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: /opt/sysarmor/bundles/tetragon-vm-recover
  install_dir: /opt/sysarmor/sensors
  policy_path: /etc/sysarmor/policies/sysarmor-managed-recover-vm.yaml
  observe_only: true
  restart: always
  max_restarts: 3
  restart_window: 8s

spool:
  path: /var/lib/sysarmor/agent/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 100ms

upload:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 100ms
EOF

echo "[e2e-agent-managed-recover-vm] starting VM topology"
bash "$HERE/start-vm.sh" >/dev/null

cd "$ENVDIR"

echo "[e2e-agent-managed-recover-vm] installing agent, systemd unit, and recovering fake bundle"
vagrant upload "$REPO/bin/sysarmor-agent" /tmp/sysarmor-agent.upload node-a >/dev/null
vagrant upload "$REPO/deployments/systemd/sysarmor-agent.service" /tmp/sysarmor-agent.service.upload node-a >/dev/null
vagrant upload "$TMP/bundle/bin/tetragon" /tmp/sysarmor-recover-tetragon node-a >/dev/null
vagrant upload "$TMP/bundle/bin/tetra" /tmp/sysarmor-recover-tetra node-a >/dev/null
vagrant upload "$TMP/bundle/manifest.json" /tmp/sysarmor-recover-manifest.json node-a >/dev/null
vagrant upload "$TMP/policy.yaml" /tmp/sysarmor-managed-recover-vm-policy.yaml node-a >/dev/null
vagrant upload "$TMP/agent.yaml" /tmp/sysarmor-managed-recover-vm-agent.yaml node-a >/dev/null

vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true; sudo rm -rf /opt/sysarmor/bundles/tetragon-vm-recover /opt/sysarmor/sensors/tetragon/vm-managed-recover /opt/sysarmor/sensors/tetragon/current /var/lib/sysarmor/agent/spool /tmp/sysarmor-managed-recover-vm-tetragon-count; sudo mkdir -p /opt/sysarmor/bundles/tetragon-vm-recover/bin /etc/sysarmor/policies /var/lib/sysarmor/agent/spool /usr/local/bin; sudo install -m 0755 /tmp/sysarmor-agent.upload /usr/local/bin/sysarmor-agent; sudo install -m 0644 /tmp/sysarmor-agent.service.upload /etc/systemd/system/sysarmor-agent.service; sudo install -m 0755 /tmp/sysarmor-recover-tetragon /opt/sysarmor/bundles/tetragon-vm-recover/bin/tetragon; sudo install -m 0755 /tmp/sysarmor-recover-tetra /opt/sysarmor/bundles/tetragon-vm-recover/bin/tetra; sudo install -m 0644 /tmp/sysarmor-recover-manifest.json /opt/sysarmor/bundles/tetragon-vm-recover/manifest.json; sudo install -m 0644 /tmp/sysarmor-managed-recover-vm-policy.yaml /etc/sysarmor/policies/sysarmor-managed-recover-vm.yaml; sudo install -m 0644 /tmp/sysarmor-managed-recover-vm-agent.yaml /etc/sysarmor/agent.yaml; sudo systemctl daemon-reload; sudo systemctl enable sysarmor-agent >/dev/null" >/dev/null

vagrant ssh mgr -c "curl -sf -X POST 'http://127.0.0.1:9443/api/v1/reset'" >/dev/null
vagrant ssh node-a -c "sudo systemctl restart sysarmor-agent" >/dev/null

wait_contains() {
  local name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 90))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-managed-recover-vm][ERROR] timeout waiting for $needle via $name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- systemd status ---" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      echo "--- agent journal ---" >&2
      vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 120 || true" >&2 2>/dev/null || true
      echo "--- count ---" >&2
      vagrant ssh node-a -c "cat /tmp/sysarmor-managed-recover-vm-tetragon-count 2>/dev/null || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

wait_contains "agent-health degraded" '"status":"degraded"' "$RESULTS/e2e-agent-managed-recover-vm.degraded.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id vm-agent-managed-recover --tenant-id default"
wait_contains "agent-health recovered" '"status":"ok"' "$RESULTS/e2e-agent-managed-recover-vm.recovered.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id vm-agent-managed-recover --tenant-id default"
wait_contains "agent-health running" '"running":true' "$RESULTS/e2e-agent-managed-recover-vm.recovered.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id vm-agent-managed-recover --tenant-id default"
wait_contains "agent-health policy" '"policy_loaded":true' "$RESULTS/e2e-agent-managed-recover-vm.recovered.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id vm-agent-managed-recover --tenant-id default"
wait_contains "agent-health restart count" '"restart_count":3' "$RESULTS/e2e-agent-managed-recover-vm.recovered.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id vm-agent-managed-recover --tenant-id default"
wait_contains "metrics" '"events_ingested":' "$RESULTS/e2e-agent-managed-recover-vm.metrics.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json metrics"

COUNT="$(vagrant ssh node-a -c "cat /tmp/sysarmor-managed-recover-vm-tetragon-count 2>/dev/null || true" 2>/dev/null | tr -d '\r' | tail -1)"
if [[ "$COUNT" != "2" ]]; then
  echo "[e2e-agent-managed-recover-vm][ERROR] fake tetragon recover count mismatch; count=${COUNT:-empty}" >&2
  vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 120 || true" >&2 2>/dev/null || true
  exit 1
fi

vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l" > "$RESULTS/e2e-agent-managed-recover-vm.systemd.txt" 2>&1 || true
vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 160" > "$RESULTS/e2e-agent-managed-recover-vm.journal.txt" 2>&1 || true

echo "[e2e-agent-managed-recover-vm] ok"
