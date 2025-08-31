# PostgreSQL 配置

此目录用于存放 PostgreSQL 数据库的配置文件。

## 配置文件

- `postgresql.conf` - PostgreSQL 主配置文件
- `pg_hba.conf` - 客户端认证配置文件
- `init.sql` - 数据库初始化脚本

## 使用方式

这些配置文件会通过 Docker volume 挂载到 PostgreSQL 容器中：

```yaml
postgres:
  volumes:
    - ./configs/postgres/postgresql.conf:/etc/postgresql/postgresql.conf:ro
    - ./configs/postgres/pg_hba.conf:/etc/postgresql/pg_hba.conf:ro
```

## 注意事项

- 配置文件修改后需要重启 PostgreSQL 服务
- 生产环境请根据实际需求调整配置参数
