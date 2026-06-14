#!/usr/bin/env bash
# attacker(10.66.0.99) provision：起 C2 监听 + 恶意脚本 HTTP 服务。
# VM 拓扑和容器拓扑共用此逻辑。
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y netcat-openbsd python3 tar

PAYLOAD_DIR=/vagrant/test/env/container/images/attacker/payloads
WORK=/opt/c2
mkdir -p "$WORK"
cp -r "$PAYLOAD_DIR"/. "$WORK"/ 2>/dev/null || true

if [[ -x "$WORK/gen-deps.sh" ]]; then bash "$WORK/gen-deps.sh" "$WORK"; fi

( cd "$WORK" && nohup python3 -m http.server 8080 >/var/log/c2-http.log 2>&1 & )
nohup bash -c 'while true; do nc -lvn 443 >>/var/log/c2-443.log 2>&1; done' >/dev/null 2>&1 &

echo "[c2] http :8080 serving $WORK ; listener :443 up"
