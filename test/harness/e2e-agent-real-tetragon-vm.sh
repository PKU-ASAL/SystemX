#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
ENVDIR="$(cd "$ROOT/env/vm" && pwd)"
RESULTS="$ROOT/.results"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
SCENARIO="${SCENARIO:-apt-staged-drop-systemd-vm}"
DUR="${DUR:-8}"
C2="${C2:-10.66.0.99}"
GAP="${GAP:-3}"

mkdir -p "$RESULTS"

cleanup() {
  (
    cd "$ENVDIR"
    vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true" >/dev/null 2>&1 || true
  )
}
trap cleanup EXIT

echo "[e2e-agent-real-tetragon-vm] starting VM topology"
bash "$HERE/start-vm.sh" >/dev/null

cd "$ENVDIR"

echo "[e2e-agent-real-tetragon-vm] installing agent systemd service with real tetra subscription"
vagrant upload "$REPO/bin/sysarmor-agent" /tmp/sysarmor-agent.upload node-a >/dev/null
vagrant upload "$REPO/deployments/systemd/sysarmor-agent.service" /tmp/sysarmor-agent.service.upload node-a >/dev/null

TETRA_PATH="$(vagrant ssh node-a -c "command -v tetra" 2>/dev/null | tr -d '\r' | tail -1)"
if [[ -z "$TETRA_PATH" ]]; then
  echo "[e2e-agent-real-tetragon-vm][ERROR] tetra is not installed in node-a VM" >&2
  exit 1
fi

vagrant ssh node-a -c "sudo systemctl start tetragon 2>/dev/null || true; sudo tetra tracingpolicy delete sysarmor-syscall-capture 2>/dev/null || true; sudo tetra tracingpolicy delete sysarmor-runtime-collection 2>/dev/null || true" >/dev/null

vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true; sudo rm -rf /var/lib/sysarmor/agent/spool-real-tetragon; sudo mkdir -p /etc/sysarmor/policies /var/lib/sysarmor/agent/spool-real-tetragon /usr/local/bin; sudo install -m 0755 /tmp/sysarmor-agent.upload /usr/local/bin/sysarmor-agent; sudo install -m 0644 /tmp/sysarmor-agent.service.upload /etc/systemd/system/sysarmor-agent.service" >/dev/null

vagrant ssh node-a -c "sudo tee /etc/sysarmor/policies/sysarmor-real-tetragon.yaml >/dev/null <<'EOF'
kinds: [EXEC, CONNECT, OPEN, WRITE, CHMOD]
EOF
sudo tee /etc/sysarmor/agent.yaml >/dev/null <<EOF
agent:
  id: vm-real-tetragon
  host_id: vm-node-a
  tenant_id: default
  token: $TOKEN
  scenario: $SCENARIO

manager:
  address: http://10.66.0.10:9443
  transport: http

sensor:
  backend: tetragon
  mode: managed
  tetra_path: $TETRA_PATH
  policy_path: /etc/sysarmor/policies/sysarmor-real-tetragon.yaml
  observe_only: true
  restart: never
  max_restarts: 1
  restart_window: 1h

spool:
  path: /var/lib/sysarmor/agent/spool-real-tetragon
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 200ms

upload:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 500ms
EOF
sudo systemctl daemon-reload
sudo systemctl enable sysarmor-agent >/dev/null" >/dev/null

vagrant ssh mgr -c "curl -sf -X POST 'http://127.0.0.1:9443/api/v1/reset?scenario=$SCENARIO' >/dev/null" >/dev/null
vagrant ssh node-a -c "sudo systemctl restart sysarmor-agent" >/dev/null

wait_contains() {
  local name="$1"
  local needle="$2"
  local out="$3"
  shift 3
  local deadline=$((SECONDS + 60))
  until "$@" >"$out" 2>"$out.err" && grep -Fq "$needle" "$out"; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-real-tetragon-vm][ERROR] timeout waiting for $needle via $name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- systemd status ---" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      echo "--- agent journal ---" >&2
      vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 120 || true" >&2 2>/dev/null || true
      echo "--- tetragon status ---" >&2
      vagrant ssh node-a -c "sudo systemctl status tetragon --no-pager -l || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

wait_contains "agent-health backend" '"backend":"tetragon"' "$RESULTS/e2e-agent-real-tetragon-vm.health.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id vm-real-tetragon --tenant-id default"
wait_contains "agent-health policy" '"policy_loaded":true' "$RESULTS/e2e-agent-real-tetragon-vm.health.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id vm-real-tetragon --tenant-id default"
wait_contains "agent-owned tracing policy" 'sysarmor-runtime-collection' "$RESULTS/e2e-agent-real-tetragon-vm.tracingpolicy.txt" \
  vagrant ssh node-a -c "sudo tetra tracingpolicy list"

echo "[e2e-agent-real-tetragon-vm] running apt-staged-drop attack"
vagrant ssh node-a -c "sudo bash -c 'GAP=$GAP C2=$C2 bash /vagrant/test/scenarios/vm/apt-staged-drop/attack.sh'" >/dev/null
sleep "$DUR"

wait_contains "scenario events" "\"scenario\":\"$SCENARIO\"" "$RESULTS/e2e-agent-real-tetragon-vm.events.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json events --scenario '$SCENARIO'"
wait_contains "endpoint payload signal" 'payload_dropped' "$RESULTS/e2e-agent-real-tetragon-vm.signals.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json signals --scenario '$SCENARIO' --layer endpoint"
wait_contains "endpoint suspicious connect signal" 'suspicious_exec_connect' "$RESULTS/e2e-agent-real-tetragon-vm.signals.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json signals --scenario '$SCENARIO' --layer endpoint"
wait_contains "cloud cross-lineage signal" 'dropped_payload_executed_and_connects' "$RESULTS/e2e-agent-real-tetragon-vm.cloud-signals.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json signals --scenario '$SCENARIO' --layer cloud"
wait_contains "incident" '"incidents":[{' "$RESULTS/e2e-agent-real-tetragon-vm.incidents.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json incidents --scenario '$SCENARIO'"

PID_BEFORE="$(vagrant ssh node-a -c "systemctl show -p MainPID --value sysarmor-agent" 2>/dev/null | tr -d '\r' | tail -1)"
if [[ -z "$PID_BEFORE" || "$PID_BEFORE" == "0" ]]; then
  echo "[e2e-agent-real-tetragon-vm][ERROR] sysarmor-agent MainPID is empty before restart" >&2
  exit 1
fi

echo "[e2e-agent-real-tetragon-vm] verifying systemd restarts real Tetragon agent"
vagrant ssh node-a -c "sudo kill -TERM $PID_BEFORE" >/dev/null
deadline=$((SECONDS + 30))
PID_AFTER=""
until [[ -n "$PID_AFTER" && "$PID_AFTER" != "0" && "$PID_AFTER" != "$PID_BEFORE" ]]; do
  if (( SECONDS >= deadline )); then
    echo "[e2e-agent-real-tetragon-vm][ERROR] systemd did not restart agent; before=$PID_BEFORE after=${PID_AFTER:-empty}" >&2
    vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
  PID_AFTER="$(vagrant ssh node-a -c "systemctl show -p MainPID --value sysarmor-agent" 2>/dev/null | tr -d '\r' | tail -1)"
done

wait_contains "agent-health after restart" '"agent_id":"vm-real-tetragon"' "$RESULTS/e2e-agent-real-tetragon-vm.health-after-restart.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --mgr 127.0.0.1:9443 --json agent-health --agent-id vm-real-tetragon --tenant-id default"

vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l" > "$RESULTS/e2e-agent-real-tetragon-vm.systemd.txt" 2>&1 || true
vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 160" > "$RESULTS/e2e-agent-real-tetragon-vm.journal.txt" 2>&1 || true

echo "[e2e-agent-real-tetragon-vm] ok"
