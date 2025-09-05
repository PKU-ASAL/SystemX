# SysArmor Flink 测试指南

## 📋 概述

本指南介绍如何测试 SysArmor 系统中的 Flink 集群（Processor 组件），包括服务端口、API 接口、数据导入和作业提交。

## 🔧 1. Processor 服务和 API

### 服务端口
- **Flink JobManager**: http://localhost:8081 (Web UI)
- **Manager API**: http://localhost:8080 (Flink 管理接口)

### 核心 API 接口

#### 集群状态检查
```bash
# 获取 Flink 集群概览
curl http://localhost:8080/api/v1/services/flink/overview | jq '.'

# 检查集群健康状态
curl http://localhost:8080/api/v1/services/flink/health | jq '.'

# 查看 TaskManager 状态
curl http://localhost:8080/api/v1/services/flink/taskmanagers | jq '.'
```

#### 作业管理
```bash
# 查看所有作业
curl http://localhost:8080/api/v1/services/flink/jobs | jq '.'

# 查看作业概览
curl http://localhost:8080/api/v1/services/flink/jobs/overview | jq '.'

# 查看特定作业详情 (需要作业ID)
curl http://localhost:8080/api/v1/services/flink/jobs/{job_id} | jq '.'
```

#### 直接访问 Flink Web UI
```bash
# 打开 Flink Web UI
open http://localhost:8081

# 或者通过 curl 查看
curl http://localhost:8081/overview
curl http://localhost:8081/jobs/overview
```

### 预期响应示例
```json
{
  "success": true,
  "data": {
    "healthy": true,
    "status": "healthy",
    "cluster_overview": {
      "slots_total": 4,
      "slots_available": 4,
      "jobs_running": 0,
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

### 作业提交示例：auditd-to-sysdig 转换器

#### 准备作业文件
```bash
# 检查作业文件是否存在
ls -la services/processor/jobs/job_auditd_to_sysdig_converter.py

# 检查作业配置
cat services/processor/configs/auditd-converter.yaml
```

#### 提交作业
```bash
# 方式1: 通过 Manager API 提交作业
curl -X POST http://localhost:8080/api/v1/services/flink/jobs/submit \
  -H "Content-Type: application/json" \
  -d '{
    "job_name": "auditd-to-sysdig-converter",
    "job_file": "/app/jobs/job_auditd_to_sysdig_converter.py",
    "config": {
      "input_topic": "sysarmor-events-test",
      "output_topic": "sysarmor-events-sysdig",
      "parallelism": 2,
      "checkpoint_interval": 60000
    }
  }'

# 方式2: 直接在 processor 容器中提交
docker exec -it processor-jobmanager flink run \
  -py /app/jobs/job_auditd_to_sysdig_converter.py \
  --input-topic sysarmor-events-test \
  --output-topic sysarmor-events-sysdig \
  --parallelism 2
```

#### 验证作业运行
```bash
# 检查作业状态
curl http://localhost:8080/api/v1/services/flink/jobs | jq '.data[] | {name: .name, state: .state, "start-time": ."start-time"}'

# 查看作业详细信息
JOB_ID=$(curl -s http://localhost:8080/api/v1/services/flink/jobs | jq -r '.data[0].jid')
curl "http://localhost:8080/api/v1/services/flink/jobs/$JOB_ID" | jq '.'

# 查看作业处理指标
curl "http://localhost:8080/api/v1/services/flink/jobs/$JOB_ID/metrics" | jq '.data'
```

#### 验证数据处理结果
```bash
# 等待作业处理数据
sleep 30

# 检查输出 topic
./kafka-tools.sh list | grep sysarmor-events-sysdig

# 查看转换后的数据
curl "http://localhost:8080/api/v1/services/kafka/topics/sysarmor-events-sysdig/messages?limit=3" | jq '.data'

# 检查 OpenSearch 中的结果
curl "http://localhost:8080/api/v1/services/opensearch/events/recent?hours=1&size=5" | jq '.data.hits.hits[] | ._source | {timestamp, evt_type, proc_name, user_name}'
```

### 作业配置说明
```yaml
# services/processor/configs/auditd-converter.yaml
job:
  name: "auditd-to-sysdig-converter"
  parallelism: 2
  checkpoint_interval: 60000
  
kafka:
  bootstrap_servers: "middleware-kafka:9092"
  input_topic: "sysarmor-events-test"
  output_topic: "sysarmor-events-sysdig"
  
processing:
  batch_size: 100
  timeout_ms: 5000
  
opensearch:
  hosts: ["indexer-opensearch:9200"]
  index_pattern: "sysarmor-events-*"
```

### 预期处理结果
转换后的 sysdig 格式事件示例：
```json
{
  "timestamp": "2025-09-05T15:30:00Z",
  "evt_type": "open",
  "evt_category": "file",
  "proc_name": "cat",
  "proc_cmdline": "cat /etc/passwd",
  "proc_pid": 5678,
  "user_name": "root",
  "user_uid": 0,
  "fd_name": "/etc/passwd",
  "fd_type": "file",
  "container_id": null,
  "k8s_pod_name": null,
  "threat_score": 25,
  "severity": "info"
}
```

## 📊 监控和故障排查

### 基本监控
```bash
# 查看容器状态
docker ps | grep processor

# 查看容器日志
docker logs processor-jobmanager --tail 50
docker logs processor-taskmanager --tail 50

# 查看资源使用
docker stats processor-jobmanager processor-taskmanager --no-stream
```

### 常见问题
1. **作业提交失败**: 检查作业文件路径和权限
2. **数据处理停滞**: 检查 Kafka 连接和 topic 配置
3. **内存不足**: 调整 TaskManager 内存配置
4. **检查点失败**: 检查存储配置和权限

---

**SysArmor Flink 测试指南** - 简化版测试流程  
**最后更新**: 2025-09-05  
**适用版本**: v1.0.0+
