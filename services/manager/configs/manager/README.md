# Manager 应用配置

此目录用于存放 Manager 应用的配置文件。

## 配置文件

- `app.yaml` - Manager 应用主配置文件
- `logging.yaml` - 日志配置文件
- `swagger.yaml` - API 文档配置

## 使用方式

这些配置文件会通过 Docker volume 挂载到 Manager 容器中：

```yaml
manager:
  volumes:
    - ./configs/manager:/app/configs:ro
```

## 配置项说明

- 服务端口配置
- 数据库连接配置
- 日志级别和格式
- API 文档设置
- 安全认证配置

## 注意事项

- 敏感信息请使用环境变量而非配置文件
- 配置文件修改后需要重启 Manager 服务
