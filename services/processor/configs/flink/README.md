# Flink 配置

此目录用于存放 Apache Flink 的配置文件。

## 配置文件

- `flink-conf.yaml` - Flink 集群配置
- `log4j.properties` - Flink 日志配置
- `masters` - JobManager 节点配置
- `workers` - TaskManager 节点配置

## 使用方式

这些配置文件会通过 Docker volume 挂载到 Flink 容器中：

```yaml
flink-jobmanager:
  volumes:
    - ./configs/flink:/opt/flink/conf/custom:ro

flink-taskmanager:
  volumes:
    - ./configs/flink:/opt/flink/conf/custom:ro
```

## 配置项说明

- **内存配置**：JobManager 和 TaskManager 内存分配
- **并行度设置**：默认并行度和最大并行度
- **检查点配置**：状态后端和检查点间隔
- **网络配置**：网络缓冲区和超时设置
- **监控配置**：指标报告和 Web UI 设置

## 注意事项

- 内存配置需要根据实际资源调整
- 检查点配置影响容错能力和性能
- 生产环境建议启用高可用配置
