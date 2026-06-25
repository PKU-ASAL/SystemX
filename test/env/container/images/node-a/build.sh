#!/usr/bin/env bash
# 良性构建步骤（真实执行，被 benign-ci-noise 反复调用）。
# 不访问 C2/IoC,不读敏感凭据,不写攻击路径；只模拟普通 CI/cache/artifact 噪声。
set -uo pipefail

mkdir -p /tmp/sysarmor-ci-cache /tmp/sysarmor-ci-workspace/build
printf 'dependency\n' >/tmp/sysarmor-ci-cache/dependency.txt
cp /tmp/sysarmor-ci-cache/dependency.txt /tmp/sysarmor-ci-workspace/build/input.txt
/bin/sh -c 'cat /tmp/sysarmor-ci-workspace/build/input.txt >/tmp/sysarmor-ci-workspace/build/output.txt'
/usr/bin/find /tmp/sysarmor-ci-workspace -maxdepth 2 -type f >/tmp/sysarmor-ci-workspace/manifest.txt
/bin/true
echo "[ci] build cycle done"
