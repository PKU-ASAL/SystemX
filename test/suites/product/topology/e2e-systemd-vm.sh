#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
VM_ENV="${SYSARMOR_VM_ENV:-${ENV:-vm-topology}}"
ENVDIR="$(cd "$ROOT/environments/$VM_ENV" && pwd)"
RESULTS="$ROOT/.results"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
PKI_DIR="${SYSARMOR_VM_MTLS_DIR:-$ROOT/.results/pki/$VM_ENV}"
AGENT_ID="vm-owned-tetragon"

mkdir -p "$RESULTS"

cleanup() {
  (
    cd "$ENVDIR"
    vagrant ssh node-a -c "sudo systemctl stop sysarmor-agent 2>/dev/null || true" >/dev/null 2>&1 || true
  )
}
trap cleanup EXIT

echo "[e2e-agent-systemd-vm] starting VM topology"
bash "$ROOT/shared/harness/start-vm.sh" "$VM_ENV" >/dev/null

cd "$ENVDIR"

echo "[e2e-agent-systemd-vm] installing agent binary, config, and systemd unit"
vagrant upload "$REPO/bin/sysarmor-agent" /tmp/sysarmor-agent.upload node-a >/dev/null
vagrant upload "$REPO/deployments/agent/systemd/sysarmor-agent.service" /tmp/sysarmor-agent.service.upload node-a >/dev/null
vagrant upload "$PKI_DIR" /tmp/sysarmor-pki.upload node-a >/dev/null

vagrant ssh node-a -c "sudo mkdir -p /etc/sysarmor/policies /etc/sysarmor/pki /var/lib/sysarmor/agent/telemetry /usr/local/bin; sudo install -m 0755 /tmp/sysarmor-agent.upload /usr/local/bin/sysarmor-agent; sudo install -m 0644 /tmp/sysarmor-agent.service.upload /etc/systemd/system/sysarmor-agent.service; sudo install -m 0644 /tmp/sysarmor-pki.upload/ca.pem /etc/sysarmor/pki/ca.pem; sudo install -m 0644 /tmp/sysarmor-pki.upload/agent.pem /etc/sysarmor/pki/agent.pem; sudo install -m 0600 /tmp/sysarmor-pki.upload/agent-key.pem /etc/sysarmor/pki/agent-key.pem" >/dev/null

vagrant ssh node-a -c "printf '%s\n' '{\"behaviors\":[\"process.exec\"],\"observe_only\":true}' | sudo tee /etc/sysarmor/policies/sysarmor-fake.yaml >/dev/null
sudo tee /etc/sysarmor/agent.yaml >/dev/null <<EOF
agent:
  id: $AGENT_ID
  host_id: vm-node-a
  tenant_id: default
  token: $TOKEN

manager:
  address: 10.66.0.10:9444
  transport: grpc
  tls_ca: /etc/sysarmor/pki/ca.pem
  tls_cert: /etc/sysarmor/pki/agent.pem
  tls_key: /etc/sysarmor/pki/agent-key.pem
  tls_server_name: sysarmor-gateway.local

sensor:
  backend: fake
  fake_startup_events: 1
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-fake.yaml
  observe_only: true
  restart: always
  max_restarts: 1
  restart_window: 1h

telemetry:
  batch_size: 256
  flush_interval: 100ms

data_plane:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 500ms
EOF
sudo systemctl daemon-reload
sudo systemctl enable sysarmor-agent >/dev/null
sudo systemctl restart sysarmor-agent" >/dev/null

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
      echo "[e2e-agent-systemd-vm][ERROR] timeout waiting for $needle via $name" >&2
      echo "--- last response ---" >&2
      cat "$out" >&2 2>/dev/null || true
      echo "--- last error ---" >&2
      cat "$out.err" >&2 2>/dev/null || true
      echo "--- systemd status ---" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      echo "--- agent journal ---" >&2
      vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 80 || true" >&2 2>/dev/null || true
      echo "--- manager log ---" >&2
      vagrant ssh mgr -c "sudo docker logs sysarmor-manager --tail 120 2>/dev/null || true" >&2 2>/dev/null || true
      echo "--- gateway log ---" >&2
      vagrant ssh mgr -c "sudo docker logs sysarmor-gateway --tail 120 2>/dev/null || true" >&2 2>/dev/null || true
      echo "--- worker log ---" >&2
      vagrant ssh mgr -c "sudo docker logs sysarmor-worker --tail 120 2>/dev/null || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
  done
}

wait_contains "agent-health" "\"agent_id\":\"$AGENT_ID\"" "$RESULTS/e2e-agent-systemd-vm.health.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id $AGENT_ID --tenant-id default"
wait_contains "agent-health sensor" '"sensor_health"' "$RESULTS/e2e-agent-systemd-vm.health.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id $AGENT_ID --tenant-id default"
wait_contains "metrics" '"events_ingested":1' "$RESULTS/e2e-agent-systemd-vm.metrics.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --manager-url 127.0.0.1:9443 --json manager metrics"
wait_contains "events" 'fake-startup' "$RESULTS/e2e-agent-systemd-vm.events.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --manager-url 127.0.0.1:9443 --json manager events list --behavior process.exec --limit 20"

read_agent_pid() {
  vagrant ssh node-a -c "systemctl show -p MainPID --value sysarmor-agent 2>/dev/null | awk '/^[0-9]+$/ { print \"PID=\" \$1; exit }'" 2>/dev/null \
    | tr -d '\r' \
    | awk -F= '/^PID=[0-9]+$/ { print $2; exit }' || true
}

wait_agent_pid() {
  local pid=""
  local deadline=$((SECONDS + 30))
  until [[ -n "$pid" && "$pid" != "0" ]]; do
    if (( SECONDS >= deadline )); then
      echo "[e2e-agent-systemd-vm][ERROR] sysarmor-agent MainPID is empty before restart" >&2
      vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
      exit 1
    fi
    sleep 1
    pid="$(read_agent_pid)"
  done
  printf '%s\n' "$pid"
}

PID_BEFORE="$(wait_agent_pid)"

echo "[e2e-agent-systemd-vm] verifying systemd restarts agent"
if ! vagrant ssh node-a -c "sudo systemctl kill -s TERM sysarmor-agent" >/dev/null; then
  echo "[e2e-agent-systemd-vm][ERROR] failed to signal sysarmor-agent via systemctl; before=$PID_BEFORE" >&2
  vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
  vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 80 || true" >&2 2>/dev/null || true
  exit 1
fi

deadline=$((SECONDS + 30))
PID_AFTER=""
until [[ -n "$PID_AFTER" && "$PID_AFTER" != "0" && "$PID_AFTER" != "$PID_BEFORE" ]]; do
  if (( SECONDS >= deadline )); then
    if [[ -z "$PID_AFTER" || "$PID_AFTER" == "0" ]]; then
      echo "[e2e-agent-systemd-vm][ERROR] sysarmor-agent MainPID is empty after restart signal; before=$PID_BEFORE after=${PID_AFTER:-empty}" >&2
    else
      echo "[e2e-agent-systemd-vm][ERROR] systemd did not restart agent; before=$PID_BEFORE after=$PID_AFTER" >&2
    fi
    vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
    vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 80 || true" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
  PID_AFTER="$(read_agent_pid)"
done

wait_contains "agent-health after systemd restart" "\"agent_id\":\"$AGENT_ID\"" "$RESULTS/e2e-agent-systemd-vm.health-after-restart.json" \
  vagrant ssh mgr -c "/tmp/sysarmorctl --manager-url 127.0.0.1:9443 --json manager health get --agent-id $AGENT_ID --tenant-id default"

vagrant ssh node-a -c "sudo systemctl status sysarmor-agent --no-pager -l" > "$RESULTS/e2e-agent-systemd-vm.systemd.txt" 2>&1 || true
vagrant ssh node-a -c "sudo journalctl -u sysarmor-agent --no-pager -n 120" > "$RESULTS/e2e-agent-systemd-vm.journal.txt" 2>&1 || true

echo "[e2e-agent-systemd-vm] ok"
