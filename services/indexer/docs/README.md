# Indexer 服务文档

此目录用于存放 Indexer 服务的相关文档。

## 文档内容

- **API 文档**：索引服务的 REST API 接口说明
- **部署指南**：服务部署和配置说明
- **运维手册**：日常运维和故障排查
- **性能调优**：索引性能优化建议

## 文档结构

```
docs/
├── api/                    # API 接口文档
│   ├── index-management.md # 索引管理接口
│   ├── search-api.md       # 搜索接口
│   └── health-check.md     # 健康检查接口
├── deployment/             # 部署文档
│   ├── installation.md     # 安装指南
│   ├── configuration.md    # 配置说明
│   └── troubleshooting.md  # 故障排查
└── operations/             # 运维文档
    ├── monitoring.md       # 监控指标
    ├── backup.md           # 备份恢复
    └── performance.md      # 性能调优
```

## 注意事项

- 文档应与代码保持同步
- 重要变更需要更新相关文档
- 建议使用 Markdown 格式编写
