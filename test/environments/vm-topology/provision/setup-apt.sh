#!/usr/bin/env bash
# Common apt bootstrap for test VMs.
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

APT_MIRROR="${SYSARMOR_APT_MIRROR:-https://mirrors.edge.kernel.org/ubuntu}"
cat >/etc/apt/sources.list <<EOF
deb $APT_MIRROR jammy main restricted universe multiverse
EOF

cat >/etc/apt/apt.conf.d/99sysarmor-retry <<'EOF'
Acquire::ForceIPv4 "true";
Acquire::Retries "3";
Acquire::http::No-Cache "true";
Acquire::https::No-Cache "true";
EOF

apt-get clean
rm -rf /var/lib/apt/lists/*
apt-get update -y

echo "[apt] sysarmor test apt bootstrap ready"
