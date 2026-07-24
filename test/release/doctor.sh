#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$HERE/common.sh"

failed=0
check_command() {
  local command="$1"
  local repair="$2"
  if command -v "$command" >/dev/null 2>&1; then
    echo "[release-doctor][OK] $command"
    return
  fi
  echo "[release-doctor][ERROR] 缺少 $command" >&2
  echo "  修复: $repair" >&2
  failed=1
}

check_command docker "安装 Docker Engine，并将当前用户加入 docker 组"
check_command curl "安装 curl（Ubuntu/Debian: sudo apt-get install curl）"
check_command python3 "安装 Python 3（Ubuntu/Debian: sudo apt-get install python3）"
check_command go "安装 go.mod 指定版本的 Go 工具链"
check_command mountpoint "安装 util-linux（Ubuntu/Debian: sudo apt-get install util-linux）"

if command -v docker >/dev/null 2>&1; then
  if docker info >/dev/null 2>&1; then
    echo "[release-doctor][OK] Docker daemon"
  else
    echo "[release-doctor][ERROR] 无法访问 Docker daemon" >&2
    echo "  修复: 启动 Docker，并执行 sudo usermod -aG docker $USER 后重新登录" >&2
    failed=1
  fi
fi

if [[ -r /sys/kernel/btf/vmlinux ]]; then
  echo "[release-doctor][OK] kernel BTF"
else
  echo "[release-doctor][ERROR] 缺少 /sys/kernel/btf/vmlinux" >&2
  echo "  修复: 使用启用 CONFIG_DEBUG_INFO_BTF 的内核，并安装对应 linux-modules-extra 包" >&2
  failed=1
fi

if [[ -d /sys/fs/bpf ]] && mountpoint -q /sys/fs/bpf; then
  echo "[release-doctor][OK] bpffs"
else
  echo "[release-doctor][ERROR] /sys/fs/bpf 未挂载" >&2
  echo "  修复: sudo mount -t bpf bpf /sys/fs/bpf" >&2
  failed=1
fi

if (( failed == 0 )); then
  install_url="$(resolve_install_url)"
  if curl -fsSL "$install_url" -o /dev/null; then
    echo "[release-doctor][OK] install URL: $install_url"
  else
    echo "[release-doctor][ERROR] 无法下载 $install_url" >&2
    echo "  修复: 检查网络，或设置 SYSARMOR_INSTALL_URL=<可访问的 install.sh URL>" >&2
    failed=1
  fi
fi

(( failed == 0 )) || exit 1
echo "[release-doctor] all checks passed"
