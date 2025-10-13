# 外部API管理方案

## 概述

SysArmor系统采用统一的外部API配置管理方案，用于管理所有第三方服务的API连接。

## 配置架构

### 1. 环境变量配置

在 `.env` 和 `.env.example` 中添加外部API配置：

```bash
# 外部API服务配置 (第三方服务)
THREAT_API_BASE_URL=http://external-threat-api:1334/api  # 威胁图谱API服务地址
THREAT_API_ENABLED=true                              # 是否启用威胁图谱功能
```

### 2. 配置管理器

创建了 `apps/ui/lib/externalApiConfig.ts` 统一管理外部API配置：

```typescript
// 支持多种环境变量格式
const config = {
  baseUrl: process.env.NEXT_PUBLIC_THREAT_API_BASE_URL || 
           process.env.THREAT_API_BASE_URL || 
           'http://external-threat-api:1334/api',
  enabled: process.env.NEXT_PUBLIC_THREAT_API_ENABLED === 'true' || 
           process.env.THREAT_API_ENABLED === 'true' || 
           true
};
```

### 3. API客户端集成

修改 `apps/ui/lib/threatApi.ts` 使用配置管理器：

```typescript
import { getThreatApiConfig, isThreatApiEnabled } from './externalApiConfig';

// 使用配置化的端点
const config = getThreatApiConfig();
const url = `${config.baseUrl}/alert/alert_chain_new_new_new`;
```

## 部署配置

### Docker构建时配置

在 `Dockerfile` 中设置构建时环境变量：

```dockerfile
# Build-time environment variables for external APIs
ARG THREAT_API_BASE_URL=http://external-threat-api:1334/api
ARG THREAT_API_ENABLED=true
ENV NEXT_PUBLIC_THREAT_API_BASE_URL=$THREAT_API_BASE_URL
ENV NEXT_PUBLIC_THREAT_API_ENABLED=$THREAT_API_ENABLED
```

### Docker Compose配置

在 `docker-compose.yml` 中传递环境变量：

```yaml
build:
  args:
    THREAT_API_BASE_URL: ${THREAT_API_BASE_URL:-http://external-threat-api:1334/api}
    THREAT_API_ENABLED: ${THREAT_API_ENABLED:-true}
environment:
  - NEXT_PUBLIC_THREAT_API_BASE_URL=${THREAT_API_BASE_URL:-http://external-threat-api:1334/api}
  - NEXT_PUBLIC_THREAT_API_ENABLED=${THREAT_API_ENABLED:-true}
```

## 使用方式

### 1. 开发环境

```bash
# .env.local
THREAT_API_BASE_URL=http://localhost:1334/api
THREAT_API_ENABLED=true
```

### 2. 生产环境

```bash
# .env
THREAT_API_BASE_URL=http://threat-api.internal:1334/api
THREAT_API_ENABLED=true
```

### 3. 禁用外部API

```bash
# .env
THREAT_API_ENABLED=false
```

## 扩展性

### 添加新的外部API

1. 在环境变量中添加配置：
```bash
ML_API_BASE_URL=http://ml-service:8080/api
ML_API_ENABLED=true
```

2. 在配置管理器中添加：
```typescript
export interface ExternalApiConfigs {
  threatApi: ExternalApiConfig;
  mlApi: ExternalApiConfig;  // 新增
}
```

3. 创建对应的API客户端类。

## 优势

1. **统一管理**: 所有外部API配置集中管理
2. **环境隔离**: 支持不同环境使用不同的API端点
3. **功能开关**: 可以通过配置启用/禁用特定功能
4. **类型安全**: TypeScript类型检查
5. **缓存支持**: 内置缓存机制提高性能
6. **错误处理**: 统一的错误处理和降级策略

## 最佳实践

1. **环境变量命名**: 使用 `SERVICE_API_BASE_URL` 格式
2. **功能开关**: 总是提供 `SERVICE_API_ENABLED` 开关
3. **默认值**: 提供合理的默认配置
4. **错误处理**: 实现优雅的降级策略
5. **缓存策略**: 合理使用缓存减少API调用
