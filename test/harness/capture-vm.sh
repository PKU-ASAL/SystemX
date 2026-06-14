#!/usr/bin/env bash
# 跑 VM 拓扑场景 + tetragon 抓内核事件 → 落盘 test/.results/<scenario>.tetragon.jsonl。
# 前提: vagrant up（env/vm/）已就绪，tetragon 在 node-a 上运行。
# 用法: capture-vm.sh <scenario> [duration_s]
#   scenario: apt-fileless-c2 | apt-staged-drop | benign-ci-noise
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"
ENVDIR="$(cd "$ROOT/env/vm" && pwd)"
RESULTS="$ROOT/.results"
S="${1:?用法: capture-vm.sh <scenario> [duration_s]}"
DUR="${2:-30}"
: "${C2:=10.66.0.99}" "${GAP:=8}" "${CYCLES:=3}"
mkdir -p "$RESULTS"

echo "[capture-vm] 确保 node-a 上 tetragon 在跑"
cd "$ENVDIR"
vagrant ssh node-a -c "sudo systemctl start tetragon 2>/dev/null || true; systemctl is-active tetragon || echo 'NOT RUNNING'" 2>/dev/null

echo "[capture-vm] 抓事件并运行场景: $S（窗口 ${DUR}s）"
vagrant ssh node-a -c "sudo bash -c '
  timeout $DUR tetra getevents -o json > /tmp/cap-$S.json 2>/dev/null &
  CAP=\$!
  sleep 3
  GAP=$GAP CYCLES=$CYCLES C2=$C2 bash /vagrant/test/scenarios/vm/$S/attack.sh
  sleep 4
  kill \$CAP 2>/dev/null; wait \$CAP 2>/dev/null || true
  echo captured=\$(wc -l < /tmp/cap-$S.json)
'"

cd "$ENVDIR"
vagrant ssh node-a -c "cat /tmp/cap-$S.json" > "$RESULTS/$S.vm.tetragon.jsonl"
echo "[capture-vm] 落盘: $RESULTS/$S.vm.tetragon.jsonl ($(wc -l < "$RESULTS/$S.vm.tetragon.jsonl") 行)"
