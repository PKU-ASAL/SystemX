#!/usr/bin/env bash
# S4 benign-ci-noise（VM 拓扑版）：完全合法的本地 CI 构建噪声。
# 断言：Incident=0。该场景不能触碰 C2/IoC、攻击落盘路径或敏感凭据。
# 注意:本脚本在 node-a VM 内执行(capture-vm.sh 通过 vagrant ssh 调用),不要再用 vagrant ssh。
set -euo pipefail
CYCLES="${CYCLES:-3}"

mkdir -p /tmp/sysarmor-ci-cache /tmp/sysarmor-ci-workspace
cat >/tmp/sysarmor-ci-cache/tool.sh <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'benign-build-tool\n' >/tmp/sysarmor-ci-workspace/tool.out
EOF
chmod +x /tmp/sysarmor-ci-cache/tool.sh

for i in $(seq 1 "$CYCLES"); do
  mkdir -p "/tmp/sysarmor-ci-workspace/build-$i"
  printf "dependency-%s\n" "$i" >"/tmp/sysarmor-ci-cache/dependency-$i.txt"
  cp "/tmp/sysarmor-ci-cache/dependency-$i.txt" "/tmp/sysarmor-ci-workspace/build-$i/input.txt"
  /bin/sh -c "cat /tmp/sysarmor-ci-workspace/build-$i/input.txt >/tmp/sysarmor-ci-workspace/build-$i/output.txt"
  /tmp/sysarmor-ci-cache/tool.sh
  /usr/bin/find /tmp/sysarmor-ci-workspace -maxdepth 2 -type f >/tmp/sysarmor-ci-workspace/manifest.txt
  /bin/true
done
echo "[benign-ci-noise] ran $CYCLES benign build cycles (vm)"
