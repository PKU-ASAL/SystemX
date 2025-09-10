# SysArmor Flink 测试指南

## 📋 概述

本指南介绍如何测试 SysArmor 系统中的 Flink 集群（Processor 组件），包括服务端口、API 接口、数据导入和作业提交。

## 🔧 1. Processor 服务和 API

### 服务架构
SysArmor Processor 基于 Apache Flink 1.18.1 + PyFlink，采用 JobManager + TaskManager 架构：

- **Flink JobManager**: http://localhost:8081 (Web UI + 作业管理)
- **Flink TaskManager**: 2个槽位，2048MB内存
- **Manager API**: http://localhost:8080 (通过Manager访问Flink)

### 核心 API 接口

#### 集群状态检查
```bash
# 通过 Manager API 获取 Flink 集群概览
curl http://localhost:8080/api/v1/services/flink/overview | jq '.'

# 检查集群健康状态
curl http://localhost:8080/api/v1/services/flink/health | jq '.'

# 查看 TaskManager 状态
curl http://localhost:8080/api/v1/services/flink/taskmanagers | jq '.'

# 直接访问 Flink Web UI
curl http://localhost:8081/overview | jq '.'
curl http://localhost:8081/jobs/overview | jq '.'
```

#### 作业管理 API
```bash
# 查看所有作业
curl http://localhost:8080/api/v1/services/flink/jobs | jq '.'

# 查看作业概览
curl http://localhost:8080/api/v1/services/flink/jobs/overview | jq '.'

# 查看特定作业详情
JOB_ID=$(curl -s http://localhost:8080/api/v1/services/flink/jobs | jq -r '.data[0].jid')
curl "http://localhost:8080/api/v1/services/flink/jobs/$JOB_ID" | jq '.'

# 查看作业指标
curl "http://localhost:8080/api/v1/services/flink/jobs/$JOB_ID/metrics" | jq '.'
```

### 可用的 Flink 作业
Processor 提供三个主要作业：

1. **基础威胁检测** (`job_rules_filter_datastream.py`)
   - DataStream API 实现
   - 内置威胁检测规则 (sudo, rm -rf, netcat等)
   - 有状态的连续威胁检测

2. **配置化威胁检测** (`job_rules_configuration_datastream.py`) 
   - 基于 YAML 配置文件的灵活规则
   - 支持动态规则加载
   - 频率基础威胁检测

3. **Auditd转换器** (`job_auditd_to_sysdig_converter.py`)
   - 将 auditd 格式转换为 sysdig 格式
   - 支持 NODLINK 标准事件类型
   - 进程树重建功能

### 预期响应示例
```json
{
  "success": true,
  "data": {
    "healthy": true,
    "status": "healthy",
    "cluster_overview": {
      "slots_total": 4,
      "slots_available": 2,
      "jobs_running": 1,
      "jobs_finished": 0,
      "jobs_cancelled": 0,
      "jobs_failed": 0
    },
    "taskmanager_overview": {
      "total_taskmanagers": 1,
      "healthy_taskmanagers": 1,
      "unhealthy_taskmanagers": 0
    }
  }
}
```

## 📥 2. 使用 Kafka Tools 导入测试数据

### 准备测试数据

#### 从服务器导出数据
```bash
cd sysarmor/scripts

# 查看远程可用的 topics
KAFKA_BROKERS=localhost:9094 ./kafka-tools.sh list

# 导出 1000 条事件数据
KAFKA_BROKERS=localhost:9094 ./kafka-tools.sh export sysarmor-agentless-b1de298c 1000
```

#### 导入到本地 Kafka
```bash
# 确保本地 middleware 服务运行

# 导入数据到本地测试 topic
./kafka-tools.sh import ./data/kafka-exports/sysarmor-agentless-b1de298c_20250905_*.jsonl sysarmor-events-test

# 验证导入结果
./kafka-tools.sh list
```

### 验证数据导入
```bash
# 检查本地 Kafka topics
curl http://localhost:8080/api/v1/services/kafka/topics | jq '.data'

# 查看测试 topic 中的消息
curl "http://localhost:8080/api/v1/services/kafka/topics/sysarmor-events-test/messages?limit=5" | jq '.data'
```

### 测试数据格式示例
导入的 JSONL 文件中每行包含一个事件，格式如下：
```json
{
  "collector_id": "12345678-abcd-efgh-ijkl-123456789012",
  "timestamp": "2025-09-05T15:30:00Z",
  "host": "test-host",
  "source": "auditd",
  "message": "type=SYSCALL msg=audit(1693420800.123:456): arch=c000003e syscall=2 success=yes exit=3 pid=5678 comm=\"cat\" exe=\"/bin/cat\"",
  "event_type": "audit",
  "severity": "info",
  "tags": ["audit", "syscall"]
}
```

## 🚀 3. 提交 Flink 作业

### 作业提交方式

SysArmor Processor 提供了便捷的 Makefile 命令来管理 Flink 作业：

#### 方式1: 使用 Makefile (推荐)
```bash
cd sysarmor

# 启动开发环境 (包含 Flink 集群)
make up-dev

# 检查 Processor 服务状态
make processor status

# 提交简单控制台测试作业
make processor submit-console

# 提交基础威胁检测作业
make processor submit-datastream

# 或提交配置化威胁检测作业 (推荐)
make processor submit-configurable

# 查看作业列表
make processor list-jobs

# 查看实时日志
make processor logs-taskmanager
```

#### 方式2: 直接容器命令
```bash
# 提交基础威胁检测作业
docker exec processor-jobmanager flink run \
  -py /opt/flink/usr_jobs/job_rules_filter_datastream.py

# 提交配置化威胁检测作业
docker exec processor-jobmanager flink run \
  -py /opt/flink/usr_jobs/job_rules_configuration_datastream.py

# 提交 auditd 转换作业
docker exec processor-jobmanager flink run \
  -py /opt/flink/usr_jobs/job_auditd_to_sysdig_converter.py
```

### 作业详细说明

#### 简单控制台测试作业 (job_simple_console_test.py)

这是一个用于验证 Flink 集群和 Kafka 数据流的基础测试作业，主要用于开发和调试阶段。

##### 作业逻辑说明

**核心功能**:
- 从多个 Kafka topics 消费实时 auditd 数据
- 解析 JSON 格式的消息并提取关键字段
- 格式化输出到控制台，便于实时监控数据流
- 统计处理的消息数量

**数据源配置**:
```python
# 消费的 Kafka Topics
topics = [
    "sysarmor-events-test",           # 测试数据 topic
    "sysarmor-agentless-b1de298c",    # racknerd-915f21b 主机数据
    "sysarmor-agentless-c289acf6"     # shenwei 主机数据
]
```

**数据处理流程**:
1. **数据消费**: 从 Kafka 集群 (49.232.13.155:9094) 消费消息
2. **JSON 解析**: 解析每条消息的 JSON 格式数据
3. **字段提取**: 提取 timestamp, host, collector_id, message 等关键字段
4. **格式化输出**: 按统一格式输出到控制台
5. **计数统计**: 维护全局消息计数器

##### 输入数据格式

作业处理的输入数据为 JSON 格式的 auditd 事件：
```json
{
  "timestamp": "2025-09-06T09:14:17.123456Z",
  "host": "racknerd-915f21b",
  "collector_id": "b1de298c-1234-5678-9abc-def012345678",
  "message": "type=SYSCALL msg=audit(1725609257.123:456): arch=c000003e syscall=2 success=yes exit=3 pid=5678 comm=\"cat\" exe=\"/bin/cat\"",
  "source": "auditd",
  "event_type": "audit"
}
```

##### 输出数据格式

控制台输出采用统一的格式化模式：
```
🔍 MESSAGE #16734 | 2025-09-06T09:14:17 | racknerd-915f21b | b1de298c | type=SYSCALL msg=audit(1725609257.123:456): arch=c000003e syscall=2...
🔍 MESSAGE #16735 | 2025-09-06T09:14:18 | shenwei | c289acf6 | type=USER_CMD msg=audit(1725609258.456:789): pid=1234 uid=0 auid=1000 ses=1 msg='cwd="/home/user"...
```

**输出格式说明**:
- `🔍 MESSAGE #N`: 消息序号，用于统计处理量
- `时间戳`: 事件发生的时间 (ISO 8601 格式)
- `主机名`: 数据来源主机 (racknerd-915f21b 或 shenwei)
- `Collector ID`: 数据收集器的短ID (前8位)
- `消息内容`: auditd 原始消息内容 (截断显示)

##### 作业提交和管理

**提交作业**:
```bash
cd sysarmor
make processor submit-console
```

**查看作业状态**:
```bash
# 查看所有运行中的作业
make processor list-jobs

# 查看实时输出日志
make processor logs-taskmanager
```

**取消作业**:
```bash
# 获取作业ID后取消
make processor cancel-job JOB_ID=<job_id>
```

##### 实际测试结果

在最近的测试中，该作业成功运行并处理了大量实时数据：

**处理统计**:
- 总处理消息数: 16,700+ 条
- 数据源: racknerd-915f21b 和 shenwei 两台主机
- 运行时长: 约30分钟
- 作业ID: 7bbe8a792295d84f8b2407bfd8017643

**数据来源分布**:
- `racknerd-915f21b`: 主要数据源，产生大量 auditd 事件
- `shenwei`: 辅助数据源，产生较少但稳定的事件流

**性能表现**:
- 实时处理延迟: < 1秒
- 消息处理速率: ~500-1000 条/分钟
- 内存使用: 稳定在 TaskManager 分配范围内
- CPU 使用: 低负载，适合长期运行

---

**SysArmor Flink 测试指南** - 更新版本  
**最后更新**: 2025-09-06  
**适用版本**: v1.0.0+
