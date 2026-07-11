#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
VM_ENV="${SYSARMOR_VM_ENV:-${ENV:-vm-endpoint}}"
ENVDIR="$(cd "$ROOT/environments/$VM_ENV" && pwd)"
AGENT_SOCK="${SYSARMOR_AGENT_SOCK:-/run/sysarmor/agent.sock}"
NODE="${SYSARMOR_VM_NODE:-node-a}"
TOKEN="${SYSARMOR_DEV_TOKEN:-dev-token}"
PKI_DIR="${SYSARMOR_VM_MTLS_DIR:-$ROOT/.results/pki/$VM_ENV}"
TETRAGON_ARCHIVE="${SYSARMOR_TETRAGON_ARCHIVE:-}"
if [[ -z "$TETRAGON_ARCHIVE" && -f "$REPO/.cache/tetragon-v1.7.0-amd64.tar.gz" ]]; then
  TETRAGON_ARCHIVE="$REPO/.cache/tetragon-v1.7.0-amd64.tar.gz"
fi
TETRAGON_BUNDLE_DIR="${SYSARMOR_TETRAGON_BUNDLE_DIR:-/opt/sysarmor/agent/bundles/tetragon}"
TETRAGON_INSTALL_DIR="${SYSARMOR_TETRAGON_INSTALL_DIR:-/opt/sysarmor/agent/sensors}"
TETRAGON_CGROUP_RATE="${SYSARMOR_TETRAGON_CGROUP_RATE:-}"
TETRAGON_PROCESS_CACHE_SIZE="${SYSARMOR_TETRAGON_PROCESS_CACHE_SIZE:-4096}"
TETRAGON_DATA_CACHE_SIZE="${SYSARMOR_TETRAGON_DATA_CACHE_SIZE:-128}"
TETRAGON_EVENT_QUEUE_SIZE="${SYSARMOR_TETRAGON_EVENT_QUEUE_SIZE:-1024}"
TETRAGON_RB_QUEUE_SIZE="${SYSARMOR_TETRAGON_RB_QUEUE_SIZE:-8192}"
FORCE_INSTALL="${SYSARMOR_BENCH_FORCE_AGENT_INSTALL:-0}"
if [[ "$VM_ENV" == "vm-topology" ]]; then
  MANAGER_CONFIG="${SYSARMOR_VM_AGENT_MANAGER_CONFIG:-  address: 10.66.0.10:9444
  transport: grpc
  tls_ca: /etc/sysarmor/pki/ca.pem
  tls_cert: /etc/sysarmor/pki/agent.pem
  tls_key: /etc/sysarmor/pki/agent-key.pem
  tls_server_name: sysarmor-gateway.local}"
else
  MANAGER_CONFIG="${SYSARMOR_VM_AGENT_MANAGER_CONFIG:-  transport: local}"
fi

if [[ ! -x "$REPO/dist/bin/sysarmor-agent" || ! -x "$REPO/dist/bin/sysarmorctl" ]]; then
  echo "[sync-agent-vm][ERROR] missing dist/bin/sysarmor-agent or dist/bin/sysarmorctl; run make build-binary first" >&2
  exit 1
fi
if [[ -n "$TETRAGON_ARCHIVE" && ! -f "$TETRAGON_ARCHIVE" ]]; then
  echo "[sync-agent-vm][ERROR] SYSARMOR_TETRAGON_ARCHIVE not found: $TETRAGON_ARCHIVE" >&2
  exit 1
fi
if [[ "$VM_ENV" == "vm-topology" && ! -f "$PKI_DIR/agent.pem" ]]; then
  SYSARMOR_GATEWAY_IPS="127.0.0.1,10.66.0.10" \
    "$REPO/tools/pki/gen-agent-plane-mtls.sh" "$PKI_DIR" default vm-owned-tetragon sysarmor-gateway.local >/dev/null
fi

cd "$ENVDIR"

echo "[sync-agent-vm] uploading current sysarmor-agent distribution to $NODE in $VM_ENV"
vagrant upload "$REPO/dist/bin/sysarmor-agent" /tmp/sysarmor-agent.upload "$NODE" >/dev/null
vagrant upload "$REPO/dist/bin/sysarmorctl" /tmp/sysarmorctl.upload "$NODE" >/dev/null
vagrant upload "$REPO/deployments" /tmp/sysarmor-deployments.upload "$NODE" >/dev/null
if [[ "$VM_ENV" == "vm-topology" ]]; then
  vagrant upload "$PKI_DIR" /tmp/sysarmor-pki.upload "$NODE" >/dev/null
fi
if [[ -n "$TETRAGON_ARCHIVE" ]]; then
  vagrant upload "$TETRAGON_ARCHIVE" /tmp/sysarmor-tetragon.upload "$NODE" >/dev/null
fi

archive_env=""
if [[ -n "$TETRAGON_ARCHIVE" ]]; then
  archive_env="SYSARMOR_TETRAGON_ARCHIVE=/tmp/sysarmor-tetragon.upload"
fi

vagrant ssh "$NODE" -c "
set -euo pipefail
sudo systemctl stop sysarmor-agent 2>/dev/null || true
sudo systemctl disable sysarmor-agent 2>/dev/null || true
sudo systemctl reset-failed sysarmor-agent 2>/dev/null || true
sudo systemctl stop tetragon 2>/dev/null || true
sudo systemctl disable tetragon 2>/dev/null || true
sudo pkill -x sysarmor-agent 2>/dev/null || true
sudo pkill -x tetragon 2>/dev/null || true
sudo pkill -x tetra 2>/dev/null || true
sudo install -m 0755 /tmp/sysarmorctl.upload /usr/local/bin/sysarmorctl
if [ '$VM_ENV' = 'vm-topology' ]; then
  sudo install -d -m 0755 /etc/sysarmor/pki
  sudo install -m 0644 /tmp/sysarmor-pki.upload/ca.pem /etc/sysarmor/pki/ca.pem
  sudo install -m 0644 /tmp/sysarmor-pki.upload/agent.pem /etc/sysarmor/pki/agent.pem
  sudo install -m 0600 /tmp/sysarmor-pki.upload/agent-key.pem /etc/sysarmor/pki/agent-key.pem
fi
sudo tee /tmp/sysarmor-owned-tetragon.yaml >/dev/null <<'EOF'
{
  \"policy_id\": \"vm-owned-tetragon-benchmark-default\",
  \"version\": 1,
  \"behaviors\": [
    {
      \"id\": \"process.exec\",
      \"enabled\": true,
      \"selectors\": {
        \"binary\": { \"prefixes\": [\"/bin/\", \"/usr/bin/\", \"/usr/local/bin/\", \"/var/lib/app/plugins\"] }
      }
    },
    {
      \"id\": \"network.connect\",
      \"enabled\": true,
      \"selectors\": {
        \"socket\": { \"families\": [\"AF_INET\"], \"addrs\": [\"10.66.0.99\"], \"ports\": [\"443\", \"8080\"] }
      }
    },
    {
      \"id\": \"file.write\",
      \"enabled\": true,
      \"selectors\": {
        \"file\": { \"prefixes\": [\"/var/lib/app/plugins\", \"/dev/shm\", \"/tmp/sysarmor\"] }
      }
    }
  ],
  \"observe_only\": true
}
EOF
sudo tee /tmp/sysarmor-agent.yaml >/dev/null <<EOF
agent:
  id: vm-owned-tetragon
  host_id: vm-node-a
  tenant_id: default
  token: $TOKEN
  label.vm_env: $VM_ENV

manager:
$MANAGER_CONFIG

control:
  socket_path: $AGENT_SOCK

sensor:
  backend: tetragon
  mode: managed
  bundle_dir: $TETRAGON_BUNDLE_DIR
  install_dir: $TETRAGON_INSTALL_DIR
  policy_path: /etc/sysarmor/policies/sysarmor-owned-tetragon.yaml
  scope:
    type: host
  observe_only: true
  restart: always
  max_restarts: 3
  restart_window: 500ms
  cgroup_rate: $TETRAGON_CGROUP_RATE
  process_cache_size: $TETRAGON_PROCESS_CACHE_SIZE
  data_cache_size: $TETRAGON_DATA_CACHE_SIZE
  event_queue_size: $TETRAGON_EVENT_QUEUE_SIZE
  rb_queue_size: $TETRAGON_RB_QUEUE_SIZE

telemetry:
  batch_size: 256
  flush_interval: 200ms

data_plane:
  retry_initial: 100ms
  retry_max: 500ms
  request_timeout: 2s

health:
  interval: 500ms
EOF
if sudo test -x '$TETRAGON_BUNDLE_DIR/bin/tetragon' && sudo test -x '$TETRAGON_BUNDLE_DIR/bin/tetra' && sudo test -f '$TETRAGON_BUNDLE_DIR/manifest.json' && [ '$FORCE_INSTALL' != '1' ] && [ -z '$archive_env' ]; then
  sudo mkdir -p /opt/sysarmor/agent/bin /etc/systemd/system /etc/sysarmor/policies /var/lib/sysarmor/agent '$TETRAGON_BUNDLE_DIR' '$TETRAGON_INSTALL_DIR'
  sudo install -m 0755 /tmp/sysarmor-agent.upload /opt/sysarmor/agent/bin/sysarmor-agent
  sudo install -m 0644 /tmp/sysarmor-deployments.upload/agent/systemd/sysarmor-agent.service /etc/systemd/system/sysarmor-agent.service
  sudo install -m 0644 /tmp/sysarmor-agent.yaml /etc/sysarmor/agent.yaml
  sudo install -m 0644 /tmp/sysarmor-owned-tetragon.yaml /etc/sysarmor/policies/sysarmor-owned-tetragon.yaml
  sudo systemctl daemon-reload 2>/dev/null || true
  sudo systemctl enable sysarmor-agent >/dev/null 2>&1 || true
  sudo rm -f /tmp/sysarmor-install-agent.log
  printf '%s\n' '[sync-agent-vm] reused existing Tetragon bundle' | sudo tee /tmp/sysarmor-install-agent.log >/dev/null
else
  sudo SYSARMOR_AGENT_BIN=/tmp/sysarmor-agent.upload \
    SYSARMOR_AGENT_CONFIG=/tmp/sysarmor-agent.yaml \
    SYSARMOR_COLLECTION_POLICY=/tmp/sysarmor-owned-tetragon.yaml \
    SYSARMOR_TETRAGON_BUNDLE_DIR='$TETRAGON_BUNDLE_DIR' \
    SYSARMOR_TETRAGON_INSTALL_DIR='$TETRAGON_INSTALL_DIR' \
    $archive_env \
    bash /tmp/sysarmor-deployments.upload/agent/install-agent.sh >/tmp/sysarmor-install-agent.log 2>&1
fi
sudo systemctl reset-failed sysarmor-agent 2>/dev/null || true
sudo systemctl restart sysarmor-agent
" >/dev/null

deadline=$((SECONDS + 90))
until vagrant ssh "$NODE" -c "sudo test -S '$AGENT_SOCK'" >/dev/null 2>&1; do
  if (( SECONDS >= deadline )); then
    echo "[sync-agent-vm][ERROR] timeout waiting for agent socket: $AGENT_SOCK" >&2
    vagrant ssh "$NODE" -c "sudo systemctl status sysarmor-agent --no-pager -l || true" >&2 2>/dev/null || true
    vagrant ssh "$NODE" -c "sudo journalctl -u sysarmor-agent --no-pager -n 120 || true" >&2 2>/dev/null || true
    exit 1
  fi
  sleep 1
done

vagrant ssh "$NODE" -c "sudo sysarmorctl --socket '$AGENT_SOCK' --json agent health >/tmp/sysarmor-sync-agent-health.json" >/dev/null
echo "[sync-agent-vm] synced current binaries and verified local control socket"
