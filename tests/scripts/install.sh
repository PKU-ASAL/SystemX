#!/bin/bash
# SysArmor 终端安装脚本 (无代理模式)
# 自动生成，收集器ID: 5ff4f634-9928-4db3-a31f-6abcef64c75e
# 生成时间: 2025-09-01 02:15:05 UTC
# 指令:  curl -s "http://localhost:8080/api/v1/scripts/setup-terminal.sh?collector_id=5ff4f634-9928-4db3-a31f-6abcef64c75e"
# 脚本功能说明:
# ============
# 
# 1. 预检查阶段 (Pre-installation Checks)
#    - 检查 root 权限：确保有足够权限修改系统配置
#    - 重复安装检查：防止重复安装或冲突安装
#    - 服务状态检查：确保 rsyslog 和 auditd 服务可用
#
# 2. 备份阶段 (Backup Phase)
#    - 备份现有配置文件：rsyslog、auditd 配置
#    - 创建时间戳备份目录：/tmp/sysarmor-backup-YYYYMMDD-HHMMSS
#    - 支持安装失败时的配置恢复
#
# 3. 配置创建阶段 (Configuration Phase)
#    - 创建 SysArmor 配置目录：/etc/sysarmor/
#    - 保存收集器元数据：collector_id, worker_address, installed_at
#    - 生成 rsyslog 转发配置：/etc/rsyslog.d/99-sysarmor.conf
#    - 生成 auditd 监控规则：/etc/audit/rules.d/sysarmor.rules
#
# 4. rsyslog 配置详解 (Rsyslog Configuration)
#    - 加载 imfile 模块：监控 /var/log/audit/audit.log 文件
#    - JSON 模板定义：结构化日志格式，包含 collector_id、timestamp 等
#    - 转发规则：将 auditd 程序的日志转发到 Vector (middleware-vector:6000)
#    - 队列配置：防止日志丢失，设置磁盘缓存和重试机制
#
# 5. auditd 规则详解 (Audit Rules Configuration)
#    - 基础设置：缓冲区大小 4096，失败模式 1，速率限制 500/秒
#    - 进程监控：监控可疑路径 (/tmp, /dev/shm, /var/tmp) 的程序执行
#    - 权限提升：监控 setuid, setgid 等系统调用
#    - 敏感文件：监控 /etc/passwd, /etc/shadow, /etc/sudoers 等文件访问
#    - 系统目录：监控 /bin, /sbin, /usr/bin, /usr/sbin 的变更
#    - 网络连接：监控 socket 和 connect 系统调用
#    - 排除规则：过滤常见系统进程，减少噪音
#
# 6. 服务重启阶段 (Service Restart Phase)
#    - 重启 auditd：加载新的审计规则
#    - 执行 augenrules --load：生成并加载规则到内核
#    - 重启 rsyslog：应用新的日志转发配置
#    - 等待服务稳定：确保服务正常启动
#
# 7. 验证阶段 (Verification Phase)
#    - 服务状态检查：确认 auditd 和 rsyslog 运行正常
#    - 网络连接测试：验证到 Vector 的网络连通性
#    - 规则加载验证：统计已加载的 SysArmor 审计规则数量
#    - 生成验证报告：提供后续检查和故障排除的命令
#
# 8. 错误处理机制 (Error Handling)
#    - 回滚函数：安装失败时自动清理已创建的文件
#    - 错误陷阱：使用 trap 捕获脚本执行错误
#    - 服务恢复：确保系统服务在失败后能正常运行
#
# 数据流向说明:
# ============
# audit 事件 → /var/log/audit/audit.log → rsyslog (imfile) → JSON 格式化 → 
# Vector (middleware-vector:6000) → Kafka Topic → 后续处理
#
# 安全考虑:
# ========
# - 最小权限原则：只监控高价值安全事件
# - 性能优化：速率限制和缓冲区控制，避免系统过载
# - 数据完整性：队列机制确保日志不丢失
# - 可恢复性：完整的备份和回滚机制

set -e

# 配置变量
WORKER_HOST="middleware-vector"
WORKER_PORT="6000"
COLLECTOR_ID="5ff4f634-9928-4db3-a31f-6abcef64c75e"
CONFIG_DIR="/etc/sysarmor"
RSYSLOG_CONFIG="/etc/rsyslog.d/99-sysarmor.conf"
AUDIT_RULES="/etc/audit/rules.d/sysarmor.rules"

echo "🚀 SysArmor 安装开始..."
echo "收集器ID: $COLLECTOR_ID"
echo "数据接收地址: $WORKER_HOST:$WORKER_PORT"
echo ""

# 权限检查
if [[ $EUID -ne 0 ]]; then
   echo "❌ 请使用 root 权限运行此脚本 (使用 sudo)"
   exit 1
fi

# 重复安装检查
if [ -f "$CONFIG_DIR/collector_id" ]; then
    EXISTING_ID=$(cat "$CONFIG_DIR/collector_id" 2>/dev/null || echo "")
    if [ "$EXISTING_ID" = "$COLLECTOR_ID" ]; then
        echo "ℹ️  收集器 $COLLECTOR_ID 已安装，将重新配置"
    else
        echo "❌ 发现其他收集器: $EXISTING_ID，请先卸载"
        exit 1
    fi
fi

# 回滚函数
rollback_installation() {
    echo "❌ 安装失败，正在回滚..."
    sudo rm -f "$RSYSLOG_CONFIG" "$AUDIT_RULES" 2>/dev/null || true
    sudo rm -rf "$CONFIG_DIR" 2>/dev/null || true
    
    # 重启rsyslog
    sudo systemctl restart rsyslog 2>/dev/null || true
    
    # 处理auditd - 某些系统不允许直接重启
    if ! sudo systemctl restart auditd 2>/dev/null; then
        echo "  - auditd无法直接重启，尝试重新加载默认规则..."
        sudo augenrules --load 2>/dev/null || true
    fi
    
    echo "✅ 回滚完成"
    exit 1
}

# 设置错误陷阱
trap rollback_installation ERR

# 备份现有配置
echo "💾 备份现有配置..."
BACKUP_DIR="/tmp/sysarmor-backup-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR"
for file in "$RSYSLOG_CONFIG" "$AUDIT_RULES" "/etc/audit/auditd.conf"; do
    [ -f "$file" ] && cp "$file" "$BACKUP_DIR/" && echo "  - 已备份 $file"
done

# 启用必要服务
echo "🔧 检查系统服务..."
for service in rsyslog auditd; do
    if ! systemctl is-enabled --quiet $service; then
        sudo systemctl enable $service
        echo "  - 已启用 $service"
    fi
    if ! systemctl is-active --quiet $service; then
        sudo systemctl start $service
        echo "  - 已启动 $service"
    fi
done

# 创建配置目录和文件
echo "📁 创建配置..."
sudo mkdir -p "$CONFIG_DIR"
echo "$COLLECTOR_ID" | sudo tee "$CONFIG_DIR/collector_id" > /dev/null
echo "$WORKER_HOST:$WORKER_PORT" | sudo tee "$CONFIG_DIR/worker_address" > /dev/null
echo "$(date -u +%Y-%m-%dT%H:%M:%SZ)" | sudo tee "$CONFIG_DIR/installed_at" > /dev/null

# 配置 rsyslog (简化版本)
echo "📡 配置日志转发..."
sudo tee "$RSYSLOG_CONFIG" > /dev/null << EOF
# SysArmor 日志转发配置
# 自动生成 - 请勿手动编辑

# 加载文件监控模块
module(load="imfile")

# 监控 audit 日志
input(type="imfile"
      File="/var/log/audit/audit.log"
      Tag="auditd"
      Severity="info"
      Facility="local6"
      readMode="2"
      freshStartTail="on"
)

# JSON 格式模板
\$template SysArmorTemplate,"{\"timestamp\":\"%timestamp:::date-rfc3339%\",\"collector_id\":\"$COLLECTOR_ID\",\"host\":\"%hostname%\",\"program\":\"%programname%\",\"message\":\"%msg:::json%\",\"event_type\":\"syslog\"}"

# 转发 audit 事件
if \$programname == 'auditd' then {
    *.* @@$WORKER_HOST:$WORKER_PORT;SysArmorTemplate
    stop
}

# 队列配置
\$ActionQueueFileName sysarmor_queue
\$ActionQueueMaxDiskSpace 50m
\$ActionQueueSaveOnShutdown on
\$ActionQueueType LinkedList
\$ActionResumeRetryCount -1
EOF

# 配置 audit 规则 (简化版本)
echo "🔍 配置审计规则..."
sudo tee "$AUDIT_RULES" > /dev/null << 'EOF'
# SysArmor audit rules for security monitoring (优化版本)
# 专注高价值安全事件，大幅减少数据量
# Generated for collector: 5ff4f634-9928-4db3-a31f-6abcef64c75e
# Auto-generated - DO NOT EDIT MANUALLY

# 删除所有现有规则
-D

# 设置缓冲区大小 (减小以降低内存使用)
-b 4096

# 设置失败模式 (0=silent, 1=printk, 2=panic)
-f 1

# 设置速率限制 (每秒最多500条消息，防止日志洪水)
-r 500

# === 高价值安全事件监控 ===

# 1. 进程执行监控 (只监控特定可疑路径和SUID程序)
-a always,exit -F arch=b64 -S execve -F exe=/tmp/* -k suspicious_activity
-a always,exit -F arch=b64 -S execve -F exe=/dev/shm/* -k suspicious_activity
-a always,exit -F arch=b64 -S execve -F exe=/var/tmp/* -k suspicious_activity
-a always,exit -F arch=b32 -S execve -F exe=/tmp/* -k suspicious_activity
-a always,exit -F arch=b32 -S execve -F exe=/dev/shm/* -k suspicious_activity
-a always,exit -F arch=b32 -S execve -F exe=/var/tmp/* -k suspicious_activity

# 2. 权限提升监控
-a always,exit -F arch=b64 -S setuid -S setgid -S setreuid -S setregid -k privilege_escalation
-a always,exit -F arch=b32 -S setuid -S setgid -S setreuid -S setregid -k privilege_escalation

# 3. 敏感文件访问监控 (只监控写入操作)
-w /etc/passwd -p wa -k sensitive_files
-w /etc/shadow -p wa -k sensitive_files
-w /etc/sudoers -p wa -k sensitive_files
-w /etc/sudoers.d/ -p wa -k sensitive_files
-w /etc/ssh/sshd_config -p wa -k sensitive_files

# 4. 系统关键目录监控 (只监控写入和属性变更)
-w /bin/ -p wa -k system_binaries
-w /sbin/ -p wa -k system_binaries
-w /usr/bin/ -p wa -k system_binaries
-w /usr/sbin/ -p wa -k system_binaries

# 5. 网络连接监控 (只监控特定端口和协议)
-a always,exit -F arch=b64 -S socket -F a0=2 -k network_connections
-a always,exit -F arch=b64 -S connect -F a0=2 -k network_connections
-a always,exit -F arch=b32 -S socket -F a0=2 -k network_connections
-a always,exit -F arch=b32 -S connect -F a0=2 -k network_connections

# 6. 用户认证和会话监控
-w /var/log/auth.log -p wa -k authentication
-w /var/log/secure -p wa -k authentication

# 7. 定时任务监控
-w /etc/crontab -p wa -k scheduled_tasks
-w /etc/cron.d/ -p wa -k scheduled_tasks
-w /var/spool/cron/ -p wa -k scheduled_tasks

# === 排除规则 - 减少噪音 ===

# 排除常见的系统进程，减少数据量
-a never,exit -F arch=b64 -S execve -F exe=/usr/bin/dpkg
-a never,exit -F arch=b64 -S execve -F exe=/usr/bin/apt
-a never,exit -F arch=b64 -S execve -F exe=/usr/bin/apt-get
-a never,exit -F arch=b64 -S execve -F exe=/bin/systemctl
-a never,exit -F arch=b64 -S execve -F exe=/usr/bin/systemctl
-a never,exit -F arch=b64 -S execve -F exe=/bin/ps
-a never,exit -F arch=b64 -S execve -F exe=/usr/bin/ps
-a never,exit -F arch=b64 -S execve -F exe=/bin/ls
-a never,exit -F arch=b64 -S execve -F exe=/usr/bin/ls

# 排除特定用户的活动 (如果有系统用户需要排除)
# -a never,user -F uid=daemon
# -a never,user -F uid=nobody

# 注意: 移除了 -e 2 (不可变模式) 以便于干净卸载
# 规则在系统重启后仍然有效，但可以在运行时修改和删除

EOF

# 重启服务 (优化顺序，处理auditd特殊情况)
echo "🔄 重启服务..."

# 处理auditd服务 - 某些系统不允许直接restart auditd
echo "  - 处理auditd服务..."
if sudo systemctl restart auditd 2>/dev/null; then
    echo "    ✅ auditd重启成功"
else
    echo "    ⚠️  auditd无法直接重启，尝试重新加载规则..."
    # 尝试重新加载规则而不重启服务
    sudo augenrules --load 2>/dev/null || true
    
    # 如果auditd未运行，尝试启动
    if ! sudo systemctl is-active --quiet auditd; then
        echo "    - 尝试启动auditd服务..."
        if sudo service auditd start 2>/dev/null || sudo /sbin/service auditd start 2>/dev/null; then
            echo "    ✅ auditd启动成功"
        else
            echo "    ⚠️  auditd启动失败，可能需要重启系统以应用审计规则"
            echo "    💡 建议: 重启系统后审计规则将自动生效"
        fi
    else
        echo "    ✅ auditd已在运行"
    fi
fi

sleep 2

# 加载审计规则
echo "  - 加载审计规则..."
if sudo augenrules --load 2>/dev/null; then
    echo "    ✅ 审计规则加载成功"
else
    echo "    ⚠️  审计规则加载失败，将在下次重启时生效"
fi

# 重启rsyslog服务
echo "  - 重启rsyslog服务..."
if sudo systemctl restart rsyslog; then
    echo "    ✅ rsyslog重启成功"
else
    echo "    ❌ rsyslog重启失败"
fi

sleep 3

# 验证安装
echo "✅ 验证安装..."
INSTALL_SUCCESS=true

# 检查服务状态
for service in auditd rsyslog; do
    if sudo systemctl is-active --quiet $service; then
        echo "  ✅ $service 运行正常"
    else
        echo "  ❌ $service 运行异常"
        INSTALL_SUCCESS=false
    fi
done

# 检查网络连接
if timeout 5 bash -c "</dev/tcp/$WORKER_HOST/$WORKER_PORT" 2>/dev/null; then
    echo "  ✅ 网络连接正常"
else
    echo "  ⚠️  无法连接到 $WORKER_HOST:$WORKER_PORT"
fi

# 检查规则加载
RULES_COUNT=$(sudo auditctl -l | grep -E "(suspicious_activity|privilege_escalation|sensitive_files|system_binaries|network_connections|authentication|scheduled_tasks)" | wc -l)
echo "  ✅ 已加载 $RULES_COUNT 条审计规则"

# 移除错误陷阱
trap - ERR

if [ "$INSTALL_SUCCESS" = true ]; then
    echo ""
    echo "🎉 SysArmor 安装成功！"
    echo ""
    echo "📋 配置信息:"
    echo "  - 收集器ID: $COLLECTOR_ID"
    echo "  - 数据接收: $WORKER_HOST:$WORKER_PORT"
    echo "  - 配置目录: $CONFIG_DIR"
    echo "  - 备份位置: $BACKUP_DIR"
    echo ""
    echo "🔍 验证命令:"
    echo "  sudo systemctl status rsyslog auditd"
    echo "  sudo auditctl -l"
    echo "  sudo tail -f /var/log/audit/audit.log"
else
    echo ""
    echo "⚠️  安装完成但存在警告，请检查上述问题"
fi