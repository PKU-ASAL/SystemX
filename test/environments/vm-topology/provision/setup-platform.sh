#!/usr/bin/env bash
# mgr VM: platform dependencies for deployment-shaped topology tests.
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y ca-certificates curl docker.io rsync

rm -f /etc/resolv.conf
cat >/etc/resolv.conf <<'EOF'
nameserver 1.1.1.1
nameserver 8.8.8.8
EOF

mkdir -p /etc/docker
cat >/etc/docker/daemon.json <<'EOF'
{
  "dns": ["1.1.1.1", "8.8.8.8"]
}
EOF

systemctl enable docker >/dev/null
systemctl daemon-reload
systemctl restart docker.socket 2>/dev/null || true
systemctl restart docker

if ! docker compose version >/dev/null 2>&1 && ! command -v docker-compose >/dev/null 2>&1; then
  apt-get install -y docker-compose-plugin || apt-get install -y docker-compose
fi

systemctl restart docker

echo "[platform] docker compose ready for sysarmor deployment stack"
