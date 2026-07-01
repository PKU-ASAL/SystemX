#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
VM_ENV="${SYSARMOR_VM_ENV:-${ENV:-vm-endpoint}}"
ENVDIR="$(cd "$ROOT/environments/$VM_ENV" && pwd)"
RESULTS="$ROOT/.results"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/sysarmor-agent-managed-vm.XXXXXX")"

mkdir -p "$RESULTS"

cleanup() {
  rm -rf "$TMP"
  (
    cd "$ENVDIR"
    vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true" >/dev/null 2>&1 || true
  )
}
trap cleanup EXIT

echo "[e2e-agent-managed-vm] building fake Tetragon bundle"
mkdir -p "$TMP/bundle/bin"
cat > "$TMP/bundle/bin/tetragon" <<'SCRIPT'
#!/usr/bin/env sh
COUNT="${SYSARMOR_TETRAGON_COUNT:-/tmp/sysarmor-managed-vm-tetragon-count}"
n=0
if [ -f "$COUNT" ]; then n="$(cat "$COUNT")"; fi
n=$((n + 1))
printf '%s' "$n" > "$COUNT"
sleep 300
SCRIPT
chmod +x "$TMP/bundle/bin/tetragon"

cat > "$TMP/bundle/bin/tetra" <<'SCRIPT'
#!/usr/bin/env sh
if [ "$1" = "getevents" ]; then
  printf '%s\n' '{"process_exec":{"process":{"pid":300,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":1,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"vm-node-a","time":"2026-06-14T10:00:00Z"}'
  sleep 300
fi
SCRIPT
chmod +x "$TMP/bundle/bin/tetra"

tetragon_sum="$(sha256sum "$TMP/bundle/bin/tetragon" | awk '{print $1}')"
tetra_sum="$(sha256sum "$TMP/bundle/bin/tetra" | awk '{print $1}')"
cat > "$TMP/bundle/manifest.json" <<EOF
{
  "version": "vm-managed",
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
  id: vm-agent-managed
  host_id: vm-node-a
  tenant_id: default
  token: $TOKEN

manager:
  address: http://10.66.0.10:9443
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: /opt/sysarmor/bundles/tetragon-vm
  install_dir: /opt/sysarmor/sensors
  policy_path: /etc/sysarmor/policies/sysarmor-managed-vm.yaml
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

spool:
  path: /var/lib/sysarmor/agent/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 100ms

data_plane:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 500ms
EOF

echo "[e2e-agent-managed-vm] starting VM topology"
bash "$ROOT/shared/harness/start-vm.sh" "$VM_ENV" >/dev/null

cd "$ENVDIR"

echo "[e2e-agent-managed-vm] installing agent, systemd unit, and fake bundle"
vagrant upload "$REPO/bin/sysarmor-agent" /tmp/sysarmor-agent.upload node-a >/dev/null
vagrant upload "$REPO/deployments/systemd/sysarmor-agent.service" /tmp/sysarmor-agent.service.upload node-a >/dev/null
vagrant upload "$TMP/bundle/bin/tetragon" /tmp/sysarmor-fake-tetragon node-a >/dev/null
vagrant upload "$TMP/bundle/bin/tetra" /tmp/sysarmor-fake-tetra node-a >/dev/null
vagrant upload "$TMP/bundle/manifest.json" /tmp/sysarmor-fake-manifest.json node-a >/dev/null
vagrant upload "$TMP/policy.yaml" /tmp/sysarmor-managed-vm-policy.yaml node-a >/dev/null
vagrant upload "$TMP/agent.yaml" /tmp/sysarmor-managed-vm-agent.yaml node-a >/dev/null

vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true; sudo rm -rf /opt/sysarmor/bundles/tetragon-vm /opt/sysarmor/sensors/tetragon/vm-managed /opt/sysarmor/sensors/tetragon/current /var/lib/sysarmor/agent/spool /tmp/sysarmor-managed-vm-tetragon-count; sudo mkdir -p /opt/sysarmor/bundles/tetragon-vm/bin /etc/sysarmor/policies /var/lib/sysarmor/agent/spool /usr/local/bin; sudo install -m 0755 /tmp/sysarmor-agent.upload /usr/local/bin/sysarmor-agent; sudo install -m 0644 /tmp/sysarmor-agent.service.upload /etc/systemd/system/sysarmor-agent.service; sudo install -m 0755 /tmp/sysarmor-fake-tetragon /opt/sysarmor/bundles/tetragon-vm/bin/tetragon; sudo install -m 0755 /tmp/sysarmor-fake-tetra /opt/sysarmor/bundles/tetragon-vm/bin/tetra; sudo install -m 0644 /tmp/sysarmor-fake-manifest.json /opt/sysarmor/bundles/tetragon-vm/manifest.json; sudo install -m 0644 /tmp/sysarmor-managed-vm-policy.yaml /etc/sysarmor/policies/sysarmor-managed-vm.yaml; sudo install -m 0644 /tmp/sysarmor-managed-vm-agent.yaml /etc/sysarmor/agent.yaml; sudo systemctl daemon-reload; sudo systemctl enable sysarmor-agent >/dev/null" >/dev/null

vagrant ssh mgr -c "curl -sf -X POST 'http://127.0.0.1:9443/api/v1/reset'" >/dev/null
vagrant ssh node-a -c "sudo systemctl restart sysarmor-agent" >/dev/null

wait_contains() {
  local name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 60))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-managed-vm][ERROR] timeout waiting for $needle via $name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- systemd status ---" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      echo "--- agent journal ---" >&2
      vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 100 || true" >&2 2>/dev/null || true
      echo "--- bundle install ---" >&2
      vagrant ssh node-a -c "find /opt/sysarmor/sensors -maxdepth 5 -type f -o -type l | sort || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

wait_contains "agent-health backend" '"backend":"tetragon"' "$RESULTS/e2e-agent-managed-vm.health.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id vm-agent-managed --tenant-id default"
wait_contains "agent-health installed" '"installed":true' "$RESULTS/e2e-agent-managed-vm.health.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id vm-agent-managed --tenant-id default"
wait_contains "agent-health policy" '"policy_loaded":true' "$RESULTS/e2e-agent-managed-vm.health.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id vm-agent-managed --tenant-id default"
wait_contains "metrics" '"events_ingested":1' "$RESULTS/e2e-agent-managed-vm.metrics.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --manager-url 127.0.0.1:9443 --json manager metrics"

COUNT="$(vagrant ssh node-a -c "cat /tmp/sysarmor-managed-vm-tetragon-count 2>/dev/null || true" 2>/dev/null | tr -d '\r' | tail -1)"
if [[ "$COUNT" != "1" ]]; then
  echo "[e2e-agent-managed-vm][ERROR] fake tetragon process did not start exactly once; count=${COUNT:-empty}" >&2
  vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 100 || true" >&2 2>/dev/null || true
  exit 1
fi

vagrant ssh node-a -c "test -x /opt/sysarmor/sensors/tetragon/current/bin/tetragon && test -x /opt/sysarmor/sensors/tetragon/current/bin/tetra" >/dev/null
vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l" > "$RESULTS/e2e-agent-managed-vm.systemd.txt" 2>&1 || true
vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 120" > "$RESULTS/e2e-agent-managed-vm.journal.txt" 2>&1 || true

echo "[e2e-agent-managed-vm] ok"
