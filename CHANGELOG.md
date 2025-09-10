# SysArmor EDR/HIDS 系统更新日志

## 📋 概述

记录 SysArmor EDR/HIDS 系统的重要更新和版本变更历史。

---

## 🔧 v0.1.1-dev - 2025-09-10

### ✅ **系统稳定性优化**
- **OpenSearch启动修复**: 解决SSL证书权限和健康检查配置问题
- **Manager API增强**: 修复Flink服务API，支持完整作业信息获取
- **目录结构优化**: 清理services/indexer冗余文件和目录
- **健康检查改进**: 优化Docker健康检查配置，提高启动成功率

### 🏗️ **架构改进**
- **SSL安全配置**: 完整的OpenSearch SSL传输层加密配置
- **证书管理统一**: 统一证书目录结构，自动化证书生成
- **依赖清理**: 移除未使用的Consul依赖，简化服务架构
- **配置一致性**: 确保Docker配置与服务配置文件完全一致

### 📊 **Flink服务增强**
- **作业信息完整性**: Manager API现在返回完整的作业信息(name、state、start-time等)
- **两阶段数据获取**: 优化API实现，先获取基础列表再获取详细信息
- **作业管理功能**: 完善作业提交、查询、取消的完整流程
- **API文档更新**: README中添加Flink作业详情API使用说明

### 🧹 **代码质量提升**
- **冗余文件清理**: 删除空目录、未使用配置文件和示例文件
- **依赖精简**: 移除python-consul等未使用依赖
- **文档完善**: 新增services/indexer/README.md完整服务文档
- **错误处理**: 改进健康检查和启动流程的错误处理

### 🔍 **问题修复**
- **Kafka KRaft模式**: 修复KRaft元数据同步混乱问题，改进make down确保数据卷清理
- **OpenSearch内存锁定**: 禁用bootstrap.memory_lock解决Docker环境权限问题
- **SSL证书访问**: 修复容器内证书文件权限和路径问题
- **健康检查超时**: 优化检查间隔和重试次数，适应OpenSearch启动时间
- **Manager数据库连接**: 修复启动时序问题，确保依赖服务就绪

---

## 🚀 v0.1-dev - 2025-09-05

### ✅ **配置系统优化**
- **环境配置精简**: 重构 `.env.example`
- **服务逻辑分类**: 按 Manager/Middleware/Processor/Indexer 逻辑分组
- **配置自动派生**: 只需设置核心 HOST，其他配置自动生成
- **Docker Compose 拆分**: 支持灵活的服务组合部署

### 📚 **文档体系重构**
- **文档精简**: 重构 README.md，专注核心逻辑介绍
- **文档分离**: 拆分部署指南和测试指南为独立文档

---

## 🔧 功能分支集成 - 2025-09-02 至 2025-09-04

### 📄 **详细文档**: 
- [HFW分支迁移](docs/migrations/HFW_MIGRATION_COMPLETE.md) - Wazuh生态集成
- [Nova分支迁移](docs/migrations/NOVA_MIGRATION_COMPLETE.md) - 双向心跳机制  
- [Dev-Zheng分支迁移](docs/migrations/DEV_ZHENG_MIGRATION_COMPLETE.md) - 开发功能增强

### 🎯 **核心成就**
- **Wazuh 生态集成**: 30+ API端点，完整SIEM功能，JWT认证
- **双向心跳机制**: Collector状态监控，智能探测，元数据管理
- **服务管理增强**: Kafka/Flink/OpenSearch完整管理API
- **监控体系**: Prometheus指标收集，健康检查，性能监控

### 🔧 **技术实现**
- **API 体系**: 80+ REST API端点，完整Swagger文档
- **数据模型**: 8,500+ 行代码，涵盖所有核心功能
- **测试覆盖**: 自动化测试脚本，集成验证
- **生产就绪**: 错误处理，性能优化，安全机制

---

**SysArmor EDR/HIDS 更新日志** - 记录系统演进历程  
**最后更新**: 2025-09-10
**维护状态**: 活跃维护 ✅
