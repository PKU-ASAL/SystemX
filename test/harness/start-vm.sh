#!/usr/bin/env bash
# 启动 VM 拓扑: vagrant up
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
REPO="$(cd "$ROOT/.." && pwd)"

echo ">>> 构建 SysArmor binaries"
make -C "$REPO" build

cd "$ROOT/env/vm"
vagrant up

echo ">>> 在 mgr VM 内启动 sysarmor-manager"
vagrant upload "$REPO/bin/sysarmor-manager" /tmp/sysarmor-manager.upload mgr >/dev/null
vagrant upload "$REPO/bin/sysarmorctl" /tmp/sysarmorctl.upload mgr >/dev/null
vagrant ssh mgr -c "pkill -f '^/tmp/sysarmor-manager( |$)' 2>/dev/null || true; mv -f /tmp/sysarmor-manager.upload /tmp/sysarmor-manager; mv -f /tmp/sysarmorctl.upload /tmp/sysarmorctl; chmod +x /tmp/sysarmor-manager /tmp/sysarmorctl; nohup /tmp/sysarmor-manager --listen 0.0.0.0:9443 --grpc-listen 0.0.0.0:9444 --store /tmp/sysarmor-manager.json >/tmp/sysarmor-manager.log 2>&1 &" >/dev/null
READY=0
for i in $(seq 1 20); do
  if vagrant ssh mgr -c "curl -sf http://127.0.0.1:9443/healthz >/dev/null" >/dev/null 2>&1; then
    READY=1
    break
  fi
  sleep 1
done
if [[ "$READY" != "1" ]]; then
  vagrant ssh mgr -c "cat /tmp/sysarmor-manager.log 2>/dev/null || true"
  echo "[start-vm][ERROR] sysarmor-manager did not become healthy"
  exit 1
fi
echo "[start-vm] done"
