#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"
ENVDIR="$(cd "$ROOT/environments/vm" && pwd)"
AGENT_SOCK="${SYSARMOR_AGENT_SOCK:-/var/run/sysarmor/agent.sock}"
NODE="${SYSARMOR_VM_NODE:-node-a}"

if [[ ! -x "$REPO/bin/sysarmor-agent" || ! -x "$REPO/bin/sysarmorctl" ]]; then
  echo "[sync-agent-vm][ERROR] missing bin/sysarmor-agent or bin/sysarmorctl; run make build first" >&2
  exit 1
fi

cd "$ENVDIR"

echo "[sync-agent-vm] uploading current sysarmor-agent and sysarmorctl to $NODE"
vagrant upload "$REPO/bin/sysarmor-agent" /tmp/sysarmor-agent.upload "$NODE" >/dev/null
vagrant upload "$REPO/bin/sysarmorctl" /tmp/sysarmorctl.upload "$NODE" >/dev/null

vagrant ssh "$NODE" -c "
set -euo pipefail
sudo install -m 0755 /tmp/sysarmor-agent.upload /usr/local/bin/sysarmor-agent
sudo install -m 0755 /tmp/sysarmorctl.upload /usr/local/bin/sysarmorctl
if sudo test -f /etc/sysarmor/agent.yaml && sudo grep -q '^upload:' /etc/sysarmor/agent.yaml; then
  sudo cp /etc/sysarmor/agent.yaml /etc/sysarmor/agent.yaml.bak-before-data-plane-sync
  sudo sed -i 's/^upload:/data_plane:/' /etc/sysarmor/agent.yaml
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
