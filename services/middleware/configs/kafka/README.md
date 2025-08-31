# Kafka 配置

此目录用于存放 Apache Kafka 的配置文件。

## 配置文件

- `server.properties` - Kafka 服务器配置
- `log4j.properties` - Kafka 日志配置
- `jmx-exporter.yml` - JMX 监控配置

## 使用方式

这些配置文件会通过 Docker volume 挂载到 Kafka 容器中：

```yaml
kafka:
  volumes:
    - ./configs/kafka:/opt/kafka/config/custom:ro
```

## 配置项说明

- **KRaft 模式配置**：无需 Zookeeper 的现代 Kafka 架构
- **日志保留策略**：数据保留时间和大小限制
- **性能优化**：网络线程、IO 线程配置
- **监控集成**：JMX 指标导出配置

## 注意事项

- 当前使用 KRaft 模式，无需 Zookeeper
- 生产环境请根据数据量调整保留策略
- 监控配置与 Prometheus 集成
