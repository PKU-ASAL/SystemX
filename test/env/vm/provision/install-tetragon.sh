#!/usr/bin/env bash
# node-a VM：从官方 release 下载 tetragon 并安装。
# 下载 URL = https://github.com/cilium/tetragon/releases/download/<ver>/tetragon-<ver>-amd64.tar.gz
# 该 tarball 包含二进制(tetragon + tetra) + BPF lib(*.o)，和未来 manager 下发 sensor 是同一模式。
set -euo pipefail
TET_VER="${TET_VER:-v1.7.0}"

echo "[provision] kernel: $(uname -r)"
if [[ ! -e /sys/kernel/btf/vmlinux ]]; then
  echo "[provision][WARN] 无 /sys/kernel/btf/vmlinux —— CO-RE 可能需要降级路径"
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y ca-certificates curl jq

# bpftool（调试用，装不上不致命）
apt-get install -y linux-tools-common "linux-tools-$(uname -r)" || \
  apt-get install -y linux-tools-common linux-tools-generic || \
  echo "[provision][WARN] 无法安装 bpftool/linux-tools，跳过"

# tetragon：从 GitHub release 下载官方 tarball（含二进制 + BPF lib）
TET_URL="https://github.com/cilium/tetragon/releases/download/${TET_VER}/tetragon-${TET_VER#v}-amd64.tar.gz"
echo "[provision] 下载 tetragon $TET_VER from $TET_URL"

if curl -fsSL "$TET_URL" -o /tmp/tetragon.tar.gz 2>/dev/null; then
  echo "[provision] 下载完成，安装..."
  # tarball 内目录结构: tetragon/ (bin) + tetragon/bpf/ (BPF objects)
  # 解压到临时目录再拷贝到标准路径
  mkdir -p /tmp/tet-install
  tar xzf /tmp/tetragon.tar.gz -C /tmp/tet-install
  # 拷贝二进制
  find /tmp/tet-install -name tetragon -type f -exec cp {} /usr/local/bin/tetragon \; 2>/dev/null || true
  find /tmp/tet-install -name tetra -type f -exec cp {} /usr/local/bin/tetra \; 2>/dev/null || true
  chmod +x /usr/local/bin/tetragon /usr/local/bin/tetra 2>/dev/null || true
  # 拷贝 BPF lib
  mkdir -p /var/lib/tetragon
  find /tmp/tet-install -name '*.o' -exec cp {} /var/lib/tetragon/ \; 2>/dev/null || true
  rm -rf /tmp/tet-install /tmp/tetragon.tar.gz
  echo "[provision] tetragon 从官方 release 安装完成"
else
  echo "[provision][WARN] 无法从 GitHub 下载 tetragon（网络受限），跳过"
  echo "[provision][HINT] 手动下载: curl -L $TET_URL | tar xz -C /tmp && cp /tmp/tetragon-*/tetragon /usr/local/bin/"
fi

# systemd 服务
mkdir -p /var/run/tetragon
cat > /etc/systemd/system/tetragon.service <<'EOF'
[Unit]
Description=Tetragon eBPF Sensor
After=network.target

[Service]
RuntimeDirectory=tetragon
ExecStartPre=/bin/rm -f /var/run/tetragon/tetragon.pid
ExecStart=/usr/local/bin/tetragon --btf /sys/kernel/btf/vmlinux
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload

if command -v tetragon >/dev/null 2>&1; then
  systemctl enable --now tetragon || echo "[provision][WARN] tetragon 服务启动失败"
  # 加载 TracingPolicy
  POLICY="/vagrant/test/env/resources/syscall-capture.yaml"
  if [[ -f "$POLICY" ]]; then
    for i in $(seq 1 15); do
      tetra tracingpolicy list >/dev/null 2>&1 && break
      sleep 2
    done
    tetra tracingpolicy add "$POLICY" 2>&1 | tail -1 || true
    echo "[provision] TracingPolicy 加载完成"
  fi
else
  echo "[provision][ERROR] tetragon 二进制不可用，服务未启动"
fi

echo "[provision] done —— tetragon 在 VM 内核上运行"
