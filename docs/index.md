# SysArmor 文档

SysArmor 是面向 Linux 的端点安全与关联分析系统。Agent 可以独立完成采集、检测和有界保存；接入管理平台后，系统增加集中策略、可靠上传、图关联、调查和响应编排能力。

## 从这里开始

| 目标 | 文档 |
|---|---|
| 运行第一个 Agent 并查看 Signal | [快速开始](quickstart.md) |
| 理解系统为什么这样设计 | [设计原则](design-principles.zh-CN.md) |
| 理解 Agent、平台和数据流 | [系统架构](architecture.md) |
| 了解当前建设重点和退出标准 | [技术路线图](roadmap.md) |
| 配置 collection、detection、telemetry、response | [策略指南](guides/policy.md) |
| 安装、注册和管理 Agent | [Agent 管理](guides/agent-management.md) |
| 调查 Event、Signal、Evidence 和 Incident | [调查指南](guides/investigation.md) |
| 部署 standalone Agent 或管理平台 | [部署指南](operations/deployment.md) |
| 升级、诊断和恢复 | [维护指南](operations/maintenance.md) |
| 查配置、API 或 CLI | [配置参考](reference/configuration.md)、[API 参考](reference/api.md)、[CLI 参考](reference/cli.md) |
| 构建、修改或测试项目 | [开发指南](development/development.md)、[测试指南](development/testing.md) |

## 三项设计原则

- **动态博弈：** collection、detection、telemetry、response 通过统一控制平面持续调整，为受约束的 Agentic 策略奠定基础。
- **效能平衡：** Event、Signal、Evidence、Incident 分层保留安全信息，在行为粒度与资源成本之间建立可测量边界。
- **端云协同：** 端侧完成低延迟过滤和检测，云侧在限定作用域内进行历史与实体图关联。

完整定义和当前/目标能力边界见[设计原则](design-principles.zh-CN.md)。

## 文档边界

公开技术文档描述可验证的当前事实和明确标注的目标能力。非公开材料不属于仓库事实来源。文档的唯一职责、事实来源和维护规则见仓库根目录的 [CATALOG](../CATALOG.md)。

protobuf 是 wire contract 的事实来源；Makefile 和 CLI `--help` 是命令事实来源；Reference 文档解释稳定字段和使用边界。
