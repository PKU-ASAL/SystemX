#!/usr/bin/env bash
# 停止 VM 拓扑: vagrant destroy + 清理 libvirt 网络
set -euo pipefail
cd "$(dirname "$0")"/../env/vm
vagrant destroy -f
for net in env0 vagrant-libvirt; do
  virsh net-destroy "$net" 2>/dev/null || true
  virsh net-undefine "$net" 2>/dev/null || true
done
echo "[stop-vm] done"
