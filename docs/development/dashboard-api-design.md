# SysArmor Dashboard API 接口设计文档

## 概述

本文档基于SysArmor系统现有的API基础设施和数据结构，设计了一套完整的Dashboard数据接口，用于提供全面的安全运营视图和系统监控能力。

## 设计原则

- **基于现有基础设施** - 充分利用OpenSearch、Kafka、Flink等现有服务
- **实时性** - 支持实时数据更新和监控
- **可扩展性** - 接口设计支持未来功能扩展
- **性能优化** - 合理的缓存和聚合策略
- **用户友好** - 直观的数据结构和响应格式

## API接口设计

### 1. 安全告警统计接口

#### 1.1 告警严重程度分布
```
GET /api/v1/dashboard/alerts/severity-distribution?timeRange=24h
```

**参数:**
- `timeRange`: 时间范围 (1h/24h/7d/30d)
- `index`: 可选，指定索引模式

**返回数据:**
```json
{
  "success": true,
  "data": {
    "critical": 15,
    "high": 42,
    "medium": 128,
    "low": 256,
    "total": 441,
    "timeRange": "24h",
    "updatedAt": "2025-09-24T17:50:00Z"
  }
}
```

**用途:** 
- Dashboard顶部关键指标卡片
- 严重程度分布饼图
- 安全态势概览

#### 1.2 事件类型分布
```
GET /api/v1/dashboard/alerts/event-types?timeRange=7d
```

**参数:**
- `timeRange`: 时间范围
- `limit`: 返回类型数量限制，默认10

**返回数据:**
```json
{
  "success": true,
  "data": {
    "file_access": 156,
    "process_execution": 89,
    "network_connection": 67,
    "system_call": 234,
    "privilege_escalation": 12,
    "suspicious_activity": 28,
    "total": 586,
    "top_types": [
      {"type": "system_call", "count": 234, "percentage": 39.9},
      {"type": "file_access", "count": 156, "percentage": 26.6}
    ]
  }
}
```

**用途:**
- 事件类型柱状图
- 攻击向量分析
- 威胁类型趋势

#### 1.3 风险评分分布
```
GET /api/v1/dashboard/alerts/risk-score-distribution?timeRange=24h
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "ranges": {
      "90-100": 8,
      "80-89": 23,
      "70-79": 45,
      "60-69": 67,
      "50-59": 89,
      "0-49": 234
    },
    "statistics": {
      "average": 45.6,
      "median": 42,
      "max": 98,
      "min": 5
    },
    "high_risk_count": 31,
    "high_risk_percentage": 6.7
  }
}
```

**用途:**
- 风险评分直方图
- 高风险事件识别
- 风险趋势分析

### 2. 系统基础设施统计接口

#### 2.1 Collector状态概览
```
GET /api/v1/dashboard/collectors/overview
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "summary": {
      "total": 15,
      "active": 12,
      "inactive": 2,
      "offline": 1,
      "error": 0
    },
    "byEnvironment": {
      "production": 8,
      "staging": 4,
      "development": 3
    },
    "byDeploymentType": {
      "agentless": 10,
      "sysarmor-stack": 3,
      "wazuh-hybrid": 2
    },
    "performance": {
      "recentlyActive": 11,
      "avgResponseTime": 245,
      "healthyPercentage": 80.0
    },
    "recentChanges": {
      "newCollectors24h": 2,
      "offlineCollectors24h": 1
    }
  }
}
```

**用途:**
- Collector状态网格视图
- 基础设施健康监控
- 部署类型分布图

#### 2.2 数据流健康状态
```
GET /api/v1/dashboard/dataflow/health
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "pipeline": {
      "raw_to_events": {
        "status": "running",
        "processed_last_hour": 1234,
        "error_rate": 0.02,
        "avg_processing_time": 125
      },
      "events_to_alerts": {
        "status": "running", 
        "processed_last_hour": 567,
        "error_rate": 0.01,
        "avg_processing_time": 89
      }
    },
    "kafka": {
      "cluster_health": "green",
      "topics": {
        "sysarmor.raw.audit": {
          "messages_per_sec": 4.7,
          "size": "2.2 KB",
          "lag": 0
        },
        "sysarmor.events.audit": {
          "messages_per_sec": 2.1,
          "size": "1.8 KB",
          "lag": 0
        },
        "sysarmor.alerts.audit": {
          "messages_per_sec": 0.8,
          "size": "0.5 KB",
          "lag": 0
        }
      }
    },
    "flink": {
      "jobs_running": 2,
      "jobs_failed": 0,
      "taskmanagers_healthy": 1,
      "slots_utilization": 50.0
    }
  }
}
```

**用途:**
- 数据流程图
- 处理性能监控
- 服务健康状态

#### 2.3 系统性能概览
```
GET /api/v1/dashboard/system/performance
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "opensearch": {
      "cluster_status": "green",
      "indices_count": 7,
      "docs_count": 15234,
      "storage_size": "45.6 MB",
      "query_performance": {
        "avg_response_time": 12,
        "queries_per_sec": 2.3,
        "slow_queries": 0
      }
    },
    "flink": {
      "cluster_status": "healthy",
      "jobs_running": 2,
      "taskmanagers": 1,
      "slots_used": 4,
      "slots_total": 8,
      "processing_rate": 156.7,
      "memory_usage": 94.5
    },
    "kafka": {
      "cluster_status": "online",
      "brokers": 1,
      "topics": 7,
      "partitions": 179,
      "messages_per_sec": 8.2,
      "disk_usage": 12.3
    },
    "database": {
      "status": "connected",
      "response_time": 184,
      "connections": 5,
      "query_performance": "good"
    }
  }
}
```

**用途:**
- 系统性能仪表盘
- 资源使用监控
- 服务可用性状态

### 3. 时间序列分析接口

#### 3.1 告警趋势分析
```
GET /api/v1/dashboard/alerts/trends?timeRange=7d&interval=1h
```

**参数:**
- `timeRange`: 时间范围 (1h/24h/7d/30d)
- `interval`: 时间间隔 (5m/1h/1d)
- `groupBy`: 分组字段 (severity/event_type/host)

**返回数据:**
```json
{
  "success": true,
  "data": {
    "timeline": [
      {
        "timestamp": "2025-09-24T10:00:00Z",
        "total": 45,
        "critical": 2,
        "high": 8,
        "medium": 15,
        "low": 20
      },
      {
        "timestamp": "2025-09-24T11:00:00Z",
        "total": 52,
        "critical": 3,
        "high": 12,
        "medium": 18,
        "low": 19
      }
    ],
    "statistics": {
      "growth_rate": "+12%",
      "peak_hour": "14:00-15:00",
      "avg_per_hour": 38.5,
      "trend": "increasing"
    }
  }
}
```

**用途:**
- 告警趋势折线图
- 安全态势变化分析
- 峰值时间识别

#### 3.2 主机活动热力图
```
GET /api/v1/dashboard/hosts/activity-heatmap?timeRange=24h
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "hosts": [
      {
        "hostname": "web-server-01",
        "alerts": 45,
        "risk_score": 78,
        "last_activity": "2025-09-24T17:45:00Z",
        "status": "high_risk",
        "collector_id": "collector-001"
      },
      {
        "hostname": "db-server-02", 
        "alerts": 23,
        "risk_score": 45,
        "last_activity": "2025-09-24T17:40:00Z",
        "status": "medium_risk",
        "collector_id": "collector-002"
      }
    ],
    "statistics": {
      "top_active_hosts": ["web-server-01", "db-server-02", "app-server-03"],
      "total_hosts": 12,
      "high_risk_hosts": 3,
      "avg_alerts_per_host": 18.5
    }
  }
}
```

**用途:**
- 主机风险热力图
- 资产安全状态监控
- 高风险主机识别

### 4. 安全洞察接口 ⚠️ **暂时不实施 - 基础设施未支持**

#### 4.1 威胁检测摘要
```
GET /api/v1/dashboard/threats/summary?timeRange=24h
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "threats": {
      "new_threats": 8,
      "resolved_threats": 12,
      "active_threats": 23,
      "total_threats": 43
    },
    "categories": {
      "malware": 5,
      "privilege_escalation": 8,
      "data_exfiltration": 3,
      "lateral_movement": 7,
      "persistence": 4,
      "defense_evasion": 6
    },
    "metrics": {
      "mttr": "2.5h",
      "detection_accuracy": 94.2,
      "false_positive_rate": 3.8
    },
    "trends": {
      "threat_growth": "+15%",
      "resolution_improvement": "+8%"
    }
  }
}
```

**用途:**
- 威胁检测仪表盘
- 安全运营效率监控
- 威胁类别分析

#### 4.2 MITRE ATT&CK覆盖分析
```
GET /api/v1/dashboard/mitre/coverage?timeRange=7d
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "coverage": {
      "techniques_detected": 23,
      "tactics_covered": 8,
      "total_techniques": 188,
      "coverage_percentage": 12.2
    },
    "top_techniques": [
      {
        "id": "T1059",
        "name": "Command and Scripting Interpreter",
        "count": 45,
        "tactic": "Execution",
        "severity": "high"
      },
      {
        "id": "T1055",
        "name": "Process Injection", 
        "count": 23,
        "tactic": "Defense Evasion",
        "severity": "critical"
      }
    ],
    "tactics_distribution": {
      "Initial Access": 3,
      "Execution": 8,
      "Persistence": 5,
      "Privilege Escalation": 4,
      "Defense Evasion": 7,
      "Credential Access": 2,
      "Discovery": 6,
      "Lateral Movement": 3
    }
  }
}
```

**用途:**
- MITRE ATT&CK热力图
- 攻击技术覆盖分析
- 威胁情报集成

### 5. 运营效率统计接口 ⚠️ **暂时不实施 - 基础设施未支持**

#### 5.1 数据处理效率
```
GET /api/v1/dashboard/processing/efficiency?timeRange=24h
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "processing": {
      "raw_events_processed": 12456,
      "events_generated": 5678,
      "alerts_generated": 567,
      "data_reduction_ratio": 95.4
    },
    "performance": {
      "processing_latency": {
        "p50": 125,
        "p95": 456,
        "p99": 789
      },
      "throughput": {
        "events_per_sec": 3.2,
        "alerts_per_sec": 0.16
      }
    },
    "quality": {
      "false_positive_rate": 3.2,
      "detection_accuracy": 96.8,
      "rule_effectiveness": 89.5
    }
  }
}
```

**用途:**
- 数据处理效率监控
- 性能优化指标
- 质量控制仪表盘

#### 5.2 实时监控指标
```
GET /api/v1/dashboard/realtime/metrics
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "current": {
      "active_connections": 15,
      "processing_rate": 156.7,
      "queue_depth": 23,
      "error_rate": 0.01
    },
    "resources": {
      "cpu_usage": 45.6,
      "memory_usage": 67.8,
      "disk_usage": 23.4,
      "network_io": 12.3
    },
    "services": {
      "opensearch": {
        "status": "green",
        "response_time": 12,
        "queries_per_sec": 2.3
      },
      "kafka": {
        "status": "online",
        "messages_per_sec": 8.2,
        "lag": 0
      },
      "flink": {
        "status": "running",
        "jobs_healthy": 2,
        "slots_utilization": 50.0
      }
    }
  }
}
```

**用途:**
- 实时系统监控
- 资源使用仪表盘
- 服务健康状态

### 6. 时间序列分析接口

#### 6.1 告警趋势分析
```
GET /api/v1/dashboard/alerts/trends?timeRange=7d&interval=1h&groupBy=severity
```

**参数:**
- `timeRange`: 时间范围
- `interval`: 时间间隔 (5m/1h/1d)
- `groupBy`: 分组字段 (severity/event_type/host)

**返回数据:**
```json
{
  "success": true,
  "data": {
    "timeline": [
      {
        "timestamp": "2025-09-24T10:00:00Z",
        "total": 45,
        "critical": 2,
        "high": 8,
        "medium": 15,
        "low": 20
      }
    ],
    "statistics": {
      "growth_rate": "+12%",
      "peak_hour": "14:00-15:00",
      "avg_per_interval": 38.5,
      "trend": "increasing",
      "volatility": "medium"
    },
    "predictions": {
      "next_hour_estimate": 48,
      "confidence": 85.2
    }
  }
}
```

**用途:**
- 告警趋势折线图
- 安全态势预测
- 异常检测

#### 6.2 主机活动热力图
```
GET /api/v1/dashboard/hosts/activity-heatmap?timeRange=24h&limit=20
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "hosts": [
      {
        "hostname": "web-server-01",
        "alerts": 45,
        "risk_score": 78,
        "last_activity": "2025-09-24T17:45:00Z",
        "status": "high_risk",
        "collector_id": "collector-001",
        "environment": "production",
        "alert_types": ["file_access", "process_execution"]
      }
    ],
    "statistics": {
      "top_active_hosts": ["web-server-01", "db-server-02", "app-server-03"],
      "total_hosts": 12,
      "high_risk_hosts": 3,
      "avg_alerts_per_host": 18.5,
      "most_common_alert_type": "system_call"
    },
    "geographic": {
      "regions": [
        {"region": "us-east-1", "hosts": 8, "alerts": 234},
        {"region": "eu-west-1", "hosts": 4, "alerts": 123}
      ]
    }
  }
}
```

**用途:**
- 主机风险热力图
- 地理分布视图
- 资产安全评估

### 7. 高级分析接口

#### 7.1 威胁情报集成
```
GET /api/v1/dashboard/threat-intelligence/summary?timeRange=24h
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "iocs": {
      "malicious_ips": 12,
      "suspicious_domains": 8,
      "known_malware_hashes": 3
    },
    "threat_actors": {
      "apt_groups": ["APT29", "Lazarus"],
      "campaigns": ["Operation XYZ"],
      "confidence": 78.5
    },
    "recommendations": [
      {
        "priority": "high",
        "action": "Block IP 192.168.1.100",
        "reason": "Multiple failed login attempts"
      }
    ]
  }
}
```

#### 7.2 合规性报告
```
GET /api/v1/dashboard/compliance/status?framework=pci-dss
```

**返回数据:**
```json
{
  "success": true,
  "data": {
    "framework": "PCI-DSS",
    "overall_score": 87.5,
    "requirements": [
      {
        "id": "2.2.4",
        "name": "Configure system security parameters",
        "status": "compliant",
        "score": 95.0,
        "last_check": "2025-09-24T17:00:00Z"
      }
    ],
    "gaps": [
      {
        "requirement": "10.2.1",
        "description": "Log all access to cardholder data",
        "severity": "medium",
        "remediation": "Enable audit logging for database access"
      }
    ]
  }
}
```

## Dashboard布局设计

### 顶部关键指标卡片 (4个)
1. **🚨 活跃告警数** - 实时Critical+High告警数量
2. **📈 24h新增告警** - 今日新增数量和增长趋势
3. **🎯 威胁检测率** - 检测准确性百分比
4. **⚡ 平均响应时间** - MTTR指标

### 中间可视化区域 (2行4列)

**第一行:**
- **告警趋势图** - 7天时间线图表 (折线图)
- **严重程度分布** - 饼图或环形图
- **事件类型分布** - 水平柱状图
- **风险评分分布** - 直方图

**第二行:**
- **主机活动热力图** - 网格热力图或地图视图
- **数据流状态** - 流程图或Sankey图
- **Collector状态** - 状态网格或仪表盘
- **系统性能** - 多指标雷达图

### 底部详细统计区域
- **最近高危告警** - 表格形式，最新10条Critical/High告警
- **热门攻击技术** - MITRE ATT&CK技术排行榜
- **问题主机排行** - 按告警数量排序的主机列表
- **处理效率统计** - 各处理阶段的性能指标

## 实现优先级

### 🔥 第一阶段 (立即实现)
**基于现有API，快速实现核心功能**

1. **告警严重程度分布** (第1.1节) - 基于现有OpenSearch聚合API
2. **告警趋势分析** (第3.1节) - 基于现有事件搜索API + 时间范围
3. **Collector状态概览** (第2.1节) - 基于现有Collector管理API
4. **系统性能概览** (第2.3节) - 基于现有健康检查API

**预计开发时间:** 2-3天
**技术要求:** 前端图表组件 + 后端聚合查询

### ⚡ 第二阶段 (后续实现)
**需要增强现有API或新增接口**

1. **事件类型分布** (第1.2节) - 需要增强OpenSearch聚合功能
2. **风险评分分布** (第1.3节) - 基于alert.risk_score字段统计
3. **主机活动热力图** (第3.2节) - 基于metadata.host字段聚合
4. **数据流健康状态** (第2.2节) - 集成Kafka和Flink性能指标

**预计开发时间:** 3-5天
**技术要求:** 后端新增聚合接口 + 前端可视化组件

### ⚠️ 暂时不实施的功能
**基础设施未支持，待后续完善**

- **第4节 - 安全洞察接口** - 需要威胁情报和MITRE ATT&CK基础设施
- **第5节 - 运营效率统计接口** - 需要性能监控和质量评估基础设施
- **第7节 - 高级分析接口** - 需要机器学习和外部集成能力

### 🎯 第三阶段 (未来扩展)
**当基础设施完善后实现**

1. **MITRE ATT&CK映射** - 需要规则引擎和技术映射
2. **威胁情报集成** - 外部威胁源API对接
3. **预测性分析** - 基于历史数据的ML模型
4. **合规性报告** - 合规框架映射和评估
5. **运营效率监控** - 数据处理质量和性能分析

**预计开发时间:** 1-2周
**技术要求:** 机器学习模型 + 外部API集成 + 复杂业务逻辑

## 技术实现建议

### 后端实现
- **使用现有OpenSearch聚合** - 充分利用Elasticsearch聚合能力
- **缓存策略** - Redis缓存热点数据，减少OpenSearch压力
- **异步处理** - 复杂统计使用后台任务处理
- **API版本控制** - 使用v1/dashboard命名空间

### 前端实现
- **图表库选择** - 继续使用Recharts或升级到更强大的图表库
- **实时更新** - WebSocket或定时轮询更新关键指标
- **响应式设计** - 适配不同屏幕尺寸的Dashboard布局
- **交互性** - 支持图表钻取和过滤联动

### 性能优化
- **数据预聚合** - 定期生成统计数据，减少实时计算
- **分页和限制** - 大数据集使用分页和合理的默认限制
- **索引优化** - 为常用查询字段创建合适的索引
- **监控告警** - 对Dashboard API性能进行监控

## 数据源映射

### OpenSearch索引
- **sysarmor-alerts-*** - 告警数据主要来源
- **sysarmor-events-*** - 事件数据补充
- **聚合查询** - 使用terms、date_histogram、stats等聚合

### Kafka Topics
- **sysarmor.raw.audit** - 原始数据统计
- **sysarmor.events.audit** - 处理后事件统计  
- **sysarmor.alerts.audit** - 告警数据统计

### 系统服务
- **Collector API** - 基础设施状态
- **Health API** - 服务健康监控
- **Flink API** - 数据处理性能
- **Kafka API** - 消息队列状态

这个设计充分利用了现有的API基础设施，能够提供全面的安全运营视图，同时保持了良好的扩展性和实用性。
