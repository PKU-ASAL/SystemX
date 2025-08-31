# Indexer 服务配置

此目录用于存放 Python 索引服务的配置文件。

## 配置文件

- `config.yaml` - 索引服务主配置文件
- `logging.yaml` - 日志配置文件
- `mapping.json` - 索引映射配置

## 使用方式

这些配置文件会通过 Docker volume 挂载到 Indexer 容器中：

```yaml
indexer:
  volumes:
    - ./configs/indexer:/app/configs:ro
```

## 配置项说明

- **OpenSearch 连接**：集群地址、认证信息
- **索引策略**：索引名称前缀、分片配置
- **数据映射**：字段类型和分析器配置
- **生命周期管理**：索引轮转和删除策略
- **性能优化**：批量写入和刷新间隔

## 注意事项

- 索引映射一旦创建不易修改
- 生产环境请配置合适的分片数量
- 定期清理过期索引以节省存储空间
