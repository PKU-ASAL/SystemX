# SysArmor 快速开始

## 🚀 快速部署

### 单机部署
```bash
git clone https://git.pku.edu.cn/oslab/sysarmor.git
cd sysarmor

# 初始化环境
make init        

# 构建并启动所有服务
make deploy

# 验证部署
make health
```

### 访问服务
- **Manager API**: http://localhost:8080
- **API 文档**: http://localhost:8080/swagger/index.html
- **Flink 监控**: http://localhost:8081
- **OpenSearch**: http://localhost:9200

## 🧪 系统测试

### 完整测试流程
```bash
# 1. 系统健康检查
./tests/test-system-health.sh

# 2. 导入测试数据
./tests/test-kafka-producer.sh sysarmor-agentless-samples.jsonl

# 3. 验证Flink处理
./tests/test-flink-processor.sh

# 4. 查看处理结果
./scripts/kafka-tools.sh export sysarmor.events.audit 10
```

### 预期结果
- **系统健康**: 19/20 测试通过
- **数据导入**: 1000条原始数据成功导入
- **Flink处理**: 8槽位集群，作业正常运行
- **数据转换**: ~1.6% 转换率 (1000条 → 16条结构化事件)

## 📊 数据流验证

### 关键Topics监控
- **sysarmor.raw.audit**: 原始auditd数据
- **sysarmor.events.audit**: Flink处理后的结构化事件
- **sysarmor.alerts**: 威胁检测告警

### 数据流效果
```bash
📊 数据流处理统计:
  📥 原始数据: 0 → 1000 (+1000)
  🔄 处理事件: 0 → 16 (+16)
  🚨 告警事件: 0 → 0 (+0)
```

## 🔧 详细测试

### 系统健康检查
```bash
# 基础健康检查
make health

# 详细系统健康测试 (20项测试)
./tests/test-system-health.sh 

# 查看按逻辑服务分组的健康状态
curl -s http://localhost:8080/api/v1/health | jq '.data.services'
```

### 数据导入测试
```bash
# 使用测试脚本导入数据
./tests/test-kafka-producer.sh sysarmor-agentless-samples.jsonl

# 或直接使用kafka-tools导入
./scripts/kafka-tools.sh import data/kafka-imports/sample.jsonl sysarmor.raw.audit

# 查看 Kafka topics 和消息数量
./scripts/kafka-tools.sh list

# 导出验证数据
./scripts/kafka-tools.sh export sysarmor.raw.audit 5
```

### Flink 流处理测试
```bash
# 1. 提交 Flink 处理作业
./tests/test-flink-processor.sh

# 2. 查看 Flink 作业状态
make processor list-jobs

# 3. 监控作业输出
docker logs sysarmor-flink-taskmanager-1 -f | grep "Processed"

# 4. 查看处理结果
./scripts/kafka-tools.sh export sysarmor.events.audit 10

# 5. 取消作业 (可选)
make processor cancel-job JOB_ID=<job-id>
```

### 服务管理测试
```bash
# Kafka 服务管理
curl -s http://localhost:8080/api/v1/services/kafka/health | jq '.'

# Flink 服务管理  
curl -s http://localhost:8080/api/v1/services/flink/health | jq '.'

# OpenSearch 服务管理
curl -s http://localhost:8080/api/v1/services/opensearch/health | jq '.'
```

## 🔧 故障排除

### 常见问题
- **Manager异常**: `docker compose restart manager`
- **Flink卡住**: 已修复，使用后台进程
- **Kafka JVM冲突**: 使用 `kafka-tools.sh` 导入

### 系统配置
- **Flink**: 8槽位，4GB内存
- **Kafka**: 1个Broker，7个Topics
- **数据目录**: `./data/kafka-imports/`

## 📈 预期输出示例

### Flink 处理日志
```
✅ Processed: open from b1de298c
✅ Processed: socket from b1de298c
✅ Processed: connect from b1de298c
```

### 系统健康检查
```
📊 测试结果汇总
总测试数: 20
通过测试: 19
失败测试: 1
```

---

**SysArmor 快速开始** - 5分钟完成部署和验证
