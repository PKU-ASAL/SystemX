# SysArmor Auditd to Sysdig Converter

## 📋 概述

Auditd到Sysdig转换器是SysArmor Processor模块的一个重要组件，用于将原始的auditd格式数据实时转换为sysdig格式，以便后续的威胁检测和分析。

## 🏗️ 架构设计

### 数据流架构
```
Kafka (Auditd数据) → Flink转换作业 → Kafka (Sysdig数据) → 威胁检测 → OpenSearch
```

**Topic映射示例**:
```
输入: sysarmor-agentless-558c01dd → 输出: sysarmor-sysdig-558c01dd
输入: sysarmor-agentless-7bb885a8 → 输出: sysarmor-sysdig-7bb885a8
```

### 转换流程
```
原始Auditd消息 → JSON解析 → Auditd解析 → 事件分组 → Sysdig转换 → 进程树重建 → 动态Topic输出
```

## 🎯 核心功能

### 1. Auditd日志解析
- **正则匹配**: 解析auditd标准格式日志
- **字段提取**: 提取系统调用、进程信息、文件路径等
- **事件分组**: 按event_id将相关记录分组

### 2. 系统调用映射
- **60+系统调用支持**: 覆盖常见的文件、进程、网络操作
- **NODLINK兼容**: 支持22种NODLINK标准事件类型
- **动态映射**: 支持自定义系统调用映射

### 3. Sysdig格式转换
- **标准字段**: evt.type, proc.name, proc.pid, fd.name等
- **事件分类**: file, process, network, other
- **时间戳处理**: 保持原始时间戳精度

### 4. 进程树重建
- **父进程查找**: 基于时间窗口的父进程命令行重建
- **进程缓存**: 内存缓存提高查找效率
- **系统进程映射**: 预定义常见系统进程

### 5. 命令行解码
- **十六进制解码**: 自动检测并解码十六进制命令行
- **容错处理**: 解码失败时返回原始字符串
- **UTF-8支持**: 正确处理中文等多字节字符

## 📁 文件结构

```
services/processor/
├── jobs/
│   └── job_auditd_to_sysdig_converter.py    # 主转换作业
├── configs/
│   └── auditd-converter.yaml               # 配置文件
├── scripts/
│   └── run_auditd_converter.py             # 启动脚本
└── README-auditd-converter.md              # 本文档
```

## ⚙️ 配置说明

### 环境变量配置
```bash
# Kafka配置
KAFKA_BOOTSTRAP_SERVERS=middleware-kafka:9092
INPUT_TOPIC=sysarmor-agentless-558c01dd      # 输入topic
OUTPUT_TOPIC=                                # 输出topic（空则自动生成）
KAFKA_GROUP_ID=sysarmor-auditd-converter-group

# Flink配置
FLINK_PARALLELISM=2                          # 并行度
FLINK_CHECKPOINT_INTERVAL=60000              # 检查点间隔

# 处理配置
PROCESS_TREE_TIME_WINDOW=60                  # 进程树重建时间窗口
PROCESS_CACHE_SIZE=10000                     # 进程缓存大小
```

### 动态Topic生成规则
```bash
# 标准格式转换
sysarmor-agentless-558c01dd → sysarmor-sysdig-558c01dd
sysarmor-agentless-7bb885a8 → sysarmor-sysdig-7bb885a8

# 非标准格式处理
custom-topic → custom-topic-sysdig
```

### 配置文件 (auditd-converter.yaml)
```yaml
kafka:
  bootstrap_servers: "middleware-kafka:9092"
  input_topic: "sysarmor-agentless-558c01dd"
  output_topic: ""  # 空则自动生成
  consumer_group: "sysarmor-auditd-converter-group"
  
  # Topic命名规则
  topic_naming:
    input_prefix: "sysarmor-agentless-"
    output_prefix: "sysarmor-sysdig-"
    auto_generate: true

flink:
  parallelism: 2
  checkpoint_interval: 60000
  checkpoint_mode: "EXACTLY_ONCE"

processing:
  process_tree:
    time_window: 60
    cache_size: 10000
  event_filter:
    supported_events: ["read", "write", "open", "execve", "connect", ...]
```

## 🚀 使用方法

### 1. 启动转换作业

#### 使用默认配置
```bash
cd /opt/flink/scripts
python3 run_auditd_converter.py
```

#### 使用自定义配置
```bash
python3 run_auditd_converter.py --config /path/to/custom-config.yaml
```

#### 覆盖特定参数
```bash
# 指定输入topic，输出topic自动生成
python3 run_auditd_converter.py \
  --input-topic sysarmor-agentless-7bb885a8 \
  --parallelism 4

# 手动指定输出topic（覆盖自动生成）
python3 run_auditd_converter.py \
  --input-topic sysarmor-agentless-custom \
  --output-topic sysarmor-sysdig-custom \
  --parallelism 4
```

#### 验证配置（不启动作业）
```bash
python3 run_auditd_converter.py --dry-run
```

### 2. 监控作业状态

#### Flink Web UI
访问 http://localhost:8081 查看作业状态、指标和日志

#### 命令行监控
```bash
# 查看作业列表
curl http://localhost:8081/jobs

# 查看特定作业详情
curl http://localhost:8081/jobs/{job_id}

# 查看作业指标
curl http://localhost:8081/jobs/{job_id}/metrics
```

### 3. 验证转换结果

#### 检查输出Topic
```bash
# 使用Kafka工具查看输出消息（使用对应的sysdig topic）
kafka-console-consumer.sh \
  --bootstrap-server middleware-kafka:9092 \
  --topic sysarmor-sysdig-558c01dd \
  --from-beginning

# 查看所有sysdig topics
kafka-topics.sh \
  --bootstrap-server middleware-kafka:9092 \
  --list | grep sysarmor-sysdig
```

#### 通过Manager API查看
```bash
# 查看转换后的事件（使用对应的sysdig topic）
curl "http://localhost:8080/api/v1/services/kafka/topics/sysarmor-sysdig-558c01dd/messages?limit=10"

# 查看所有sysdig相关的topics
curl "http://localhost:8080/api/v1/services/kafka/topics?search=sysarmor-sysdig"
```

## 📊 支持的事件类型

### NODLINK标准事件类型 (22种)
```
文件操作: read, readv, write, writev, open, openat, fcntl, rmdir, rename, chmod
进程操作: execve, clone, fork, pipe
网络操作: socket, connect, accept, sendmsg, recvmsg, recvfrom, send, sendto
```

### 系统调用映射示例
```python
SYSCALL_MAP = {
    0: "read",      1: "write",     2: "open",      3: "close",
    41: "socket",   42: "connect",  43: "accept",   56: "clone",
    57: "fork",     59: "execve",   257: "openat",  ...
}
```

## 🔍 数据格式

### 输入格式 (Auditd)
```json
{
  "message": "type=SYSCALL msg=audit(1755378295.400:60973332): arch=c000003e syscall=2 success=yes exit=3 ppid=24710 pid=19994 comm=\"sshd\" exe=\"/usr/sbin/sshd\"",
  "timestamp": "2025-08-16T17:04:58.632055-04:00",
  "host": "racknerd-89088b0"
}
```

### 输出格式 (Sysdig)
```json
{
  "evt.num": 60973332,
  "evt.time": 1755378295.400,
  "evt.type": "open",
  "evt.category": "file",
  "proc.name": "sshd",
  "proc.exe": "/usr/sbin/sshd",
  "proc.cmdline": "/usr/sbin/sshd -D",
  "proc.pid": 19994,
  "proc.ppid": 24710,
  "proc.pcmdline": "/usr/sbin/sshd -D",
  "fd.name": "/etc/ssh/ssh_host_ed25519_key",
  "host": "racknerd-89088b0",
  "is_warn": false
}
```

## 🛠️ 开发和调试

### 本地开发
```bash
# 设置Python路径
export PYTHONPATH=/opt/flink/usr_jobs:$PYTHONPATH

# 直接运行转换作业
python3 job_auditd_to_sysdig_converter.py
```

### 调试模式
```bash
# 启用详细日志
export FLINK_LOG_LEVEL=DEBUG
python3 run_auditd_converter.py
```

### 单元测试
```bash
# 运行转换器测试
python3 -m pytest tests/test_auditd_converter.py -v
```

## 📈 性能优化

### 1. 并行度调优
- **CPU密集型**: 设置并行度为CPU核心数
- **I/O密集型**: 可以设置更高的并行度
- **内存限制**: 考虑TaskManager内存大小

### 2. 缓存优化
- **进程缓存大小**: 根据系统进程数量调整
- **时间窗口**: 平衡准确性和性能
- **内存使用**: 监控堆内存使用情况

### 3. Kafka优化
- **批处理大小**: 调整batch.size和linger.ms
- **压缩**: 使用snappy压缩减少网络传输
- **分区**: 合理设置Topic分区数

## 🚨 故障排查

### 常见问题

#### 1. 作业启动失败
```bash
# 检查Kafka连接
curl http://localhost:8080/api/v1/services/kafka/test-connection

# 检查Topic是否存在
curl http://localhost:8080/api/v1/services/kafka/topics
```

#### 2. 转换率低
- 检查输入数据格式是否正确
- 验证系统调用映射是否完整
- 查看Flink作业日志中的警告信息

#### 3. 内存不足
- 增加TaskManager内存配置
- 减少进程缓存大小
- 调整并行度

#### 4. 进程树重建失败
- 检查时间窗口设置
- 验证进程缓存配置
- 查看系统进程映射

### 日志分析
```bash
# 查看JobManager日志
docker logs processor-jobmanager

# 查看TaskManager日志
docker logs processor-taskmanager

# 查看转换作业特定日志
docker logs processor-jobmanager | grep "AuditdToSysdigConverter"
```

## 🔄 集成说明

### 与现有威胁检测的集成
1. **数据流**: 转换后的sysdig数据可以直接用于现有的威胁检测规则
2. **Topic配置**: 威胁检测作业可以订阅sysdig输出Topic
3. **格式兼容**: 输出格式与标准sysdig格式完全兼容

### 与NODLINK算法的集成
1. **事件类型**: 支持NODLINK要求的22种事件类型
2. **字段映射**: 包含proc.pcmdline等NODLINK必需字段
3. **数据质量**: 进程树重建确保数据完整性

## 📚 参考资料

- [Auditd日志格式文档](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/security_guide/sec-understanding_audit_log_files)
- [Sysdig事件格式规范](https://github.com/draios/sysdig/wiki/Sysdig-User-Guide)
- [NODLINK算法论文](https://example.com/nodlink-paper)
- [Apache Flink DataStream API](https://nightlies.apache.org/flink/flink-docs-release-1.18/docs/dev/datastream/overview/)

---

**版本**: v1.0  
**最后更新**: 2025-01-02  
**维护团队**: SysArmor Processor Team
