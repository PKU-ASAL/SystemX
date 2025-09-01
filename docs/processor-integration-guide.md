# SysArmor Manager 与 Processor 集成指南

## 📋 概述

本文档描述了 Manager 如何与 Processor 模块集成，获取 Flink 作业状态、指标和配置信息。

## 🔗 Processor 接口能力

### Flink REST API 端点

Processor 模块通过 Flink JobManager 暴露标准的 REST API（端口 8081），Manager 可以直接调用这些接口：

**Base URL**: `http://processor-jobmanager:8081`

### 1. 集群和作业管理

#### 获取集群概览
```http
GET /overview
```
**响应示例**:
```json
{
  "taskmanagers": 2,
  "slots-total": 4,
  "slots-available": 2,
  "jobs-running": 1,
  "jobs-finished": 0,
  "jobs-cancelled": 0,
  "jobs-failed": 0
}
```

#### 获取所有作业列表
```http
GET /jobs
```
**响应示例**:
```json
{
  "jobs": [
    {
      "id": "a1b2c3d4e5f6",
      "name": "Configurable Threat Detection",
      "state": "RUNNING",
      "start-time": 1693478400000,
      "end-time": -1,
      "duration": 3600000,
      "last-modification": 1693482000000
    }
  ]
}
```

#### 获取特定作业详情
```http
GET /jobs/{job-id}
```
**响应示例**:
```json
{
  "jid": "a1b2c3d4e5f6",
  "name": "Configurable Threat Detection",
  "state": "RUNNING",
  "start-time": 1693478400000,
  "end-time": -1,
  "duration": 3600000,
  "now": 1693482000000,
  "timestamps": {
    "CREATED": 1693478400000,
    "RUNNING": 1693478401000
  },
  "vertices": [
    {
      "id": "vertex1",
      "name": "Source: Kafka Consumer",
      "parallelism": 2,
      "status": "RUNNING"
    },
    {
      "id": "vertex2", 
      "name": "Threat Detection Process",
      "parallelism": 2,
      "status": "RUNNING"
    }
  ],
  "status-counts": {
    "CREATED": 0,
    "SCHEDULED": 0,
    "DEPLOYING": 0,
    "RUNNING": 4,
    "FINISHED": 0,
    "CANCELING": 0,
    "CANCELED": 0,
    "FAILED": 0
  },
  "plan": {
    "jid": "a1b2c3d4e5f6",
    "name": "Configurable Threat Detection",
    "nodes": [...]
  }
}
```

### 2. 作业指标和监控

#### 获取作业指标
```http
GET /jobs/{job-id}/metrics
```

#### 获取作业顶点指标
```http
GET /jobs/{job-id}/vertices/{vertex-id}/metrics
```

#### 获取作业异常信息
```http
GET /jobs/{job-id}/exceptions
```

### 3. TaskManager 管理

#### 获取所有 TaskManager
```http
GET /taskmanagers
```
**响应示例**:
```json
{
  "taskmanagers": [
    {
      "id": "tm1",
      "path": "akka.tcp://flink@taskmanager1:6122/user/rpc/taskmanager_0",
      "dataPort": 6121,
      "jmxPort": -1,
      "timeSinceLastHeartbeat": 1000,
      "slotsNumber": 2,
      "freeSlots": 1,
      "totalResource": {
        "cpuCores": 2.0,
        "taskHeapMemory": 1073741824,
        "taskOffHeapMemory": 134217728,
        "managedMemory": 536870912,
        "networkMemory": 134217728
      },
      "freeResource": {
        "cpuCores": 1.0,
        "taskHeapMemory": 536870912,
        "taskOffHeapMemory": 67108864,
        "managedMemory": 268435456,
        "networkMemory": 67108864
      },
      "hardware": {
        "cpuCores": 4,
        "physicalMemory": 8589934592,
        "freeMemory": 4294967296,
        "managedMemory": 536870912
      }
    }
  ]
}
```

### 4. 配置和日志

#### 获取 Flink 配置
```http
GET /config
```

#### 获取作业配置
```http
GET /jobs/{job-id}/config
```

#### 获取日志列表
```http
GET /taskmanagers/{taskmanager-id}/logs
```

## 🛠️ Manager 集成建议

### 建议在 Manager 中添加以下接口：

### 1. Processor 状态查询接口

```http
GET /api/v1/processor/overview           # 获取 Processor 集群概览
GET /api/v1/processor/jobs               # 获取所有作业列表
GET /api/v1/processor/jobs/{job-id}      # 获取特定作业详情
GET /api/v1/processor/jobs/{job-id}/metrics  # 获取作业指标
GET /api/v1/processor/taskmanagers      # 获取 TaskManager 状态
```

### 2. 实现示例

在 Manager 中创建 `ProcessorHandler`:

```go
// ProcessorHandler Processor 管理处理器
type ProcessorHandler struct {
    flinkBaseURL string
}

// NewProcessorHandler 创建 Processor 管理处理器
func NewProcessorHandler(flinkBaseURL string) *ProcessorHandler {
    return &ProcessorHandler{
        flinkBaseURL: flinkBaseURL,
    }
}

// GetOverview 获取 Processor 集群概览
func (h *ProcessorHandler) GetOverview(c *gin.Context) {
    resp, err := http.Get(h.flinkBaseURL + "/overview")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Failed to get processor overview: " + err.Error(),
        })
        return
    }
    defer resp.Body.Close()
    
    var overview map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&overview); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Failed to parse response: " + err.Error(),
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    overview,
    })
}

// GetJobs 获取所有作业列表
func (h *ProcessorHandler) GetJobs(c *gin.Context) {
    resp, err := http.Get(h.flinkBaseURL + "/jobs")
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Failed to get jobs: " + err.Error(),
        })
        return
    }
    defer resp.Body.Close()
    
    var jobs map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&jobs); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Failed to parse response: " + err.Error(),
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    jobs,
    })
}

// GetJobDetails 获取特定作业详情
func (h *ProcessorHandler) GetJobDetails(c *gin.Context) {
    jobID := c.Param("job_id")
    if jobID == "" {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "error":   "job_id is required",
        })
        return
    }
    
    resp, err := http.Get(h.flinkBaseURL + "/jobs/" + jobID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Failed to get job details: " + err.Error(),
        })
        return
    }
    defer resp.Body.Close()
    
    var jobDetails map[string]interface{}
    if err := json.NewDecoder(resp.Body).Decode(&jobDetails); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Failed to parse response: " + err.Error(),
        })
        return
    }
    
    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    jobDetails,
    })
}
```

### 3. 路由注册

在 `main.go` 中添加路由：

```go
// Processor 管理路由
processorHandler := handlers.NewProcessorHandler("http://processor-jobmanager:8081")
processor := api.Group("/processor")
{
    processor.GET("/overview", processorHandler.GetOverview)
    processor.GET("/jobs", processorHandler.GetJobs)
    processor.GET("/jobs/:job_id", processorHandler.GetJobDetails)
    processor.GET("/jobs/:job_id/metrics", processorHandler.GetJobMetrics)
    processor.GET("/taskmanagers", processorHandler.GetTaskManagers)
}
```

## 📊 可获取的关键信息

### 1. 集群状态
- TaskManager 数量和状态
- 可用/总计 slot 数量
- 运行/完成/失败作业统计

### 2. 作业信息
- 作业 ID、名称、状态
- 启动时间、运行时长
- 并行度设置
- 作业拓扑结构

### 3. 性能指标
- 吞吐量（records/sec）
- 延迟指标
- 背压状态
- 资源使用率

### 4. 威胁检测状态
- 威胁检测规则加载状态
- 处理的事件数量
- 检测到的威胁数量
- 规则匹配统计

## 🚀 使用示例

### 获取 Processor 集群概览
```bash
curl http://localhost:8080/api/v1/processor/overview
```

### 获取所有作业状态
```bash
curl http://localhost:8080/api/v1/processor/jobs
```

### 获取特定作业详情
```bash
curl http://localhost:8080/api/v1/processor/jobs/a1b2c3d4e5f6
```

### 获取 TaskManager 状态
```bash
curl http://localhost:8080/api/v1/processor/taskmanagers
```

## 🔍 监控建议

### 1. 关键指标监控
- 作业运行状态（RUNNING/FAILED/FINISHED）
- TaskManager 健康状态
- 处理延迟和吞吐量
- 资源使用率

### 2. 告警规则
- 作业状态异常告警
- TaskManager 离线告警
- 处理延迟过高告警
- 资源使用率过高告警

### 3. 健康检查集成
将 Processor 状态集成到 Manager 的健康检查系统中：

```go
// 在健康检查中添加 Processor 状态
func (h *HealthHandler) GetComprehensiveHealth(c *gin.Context) {
    // ... 其他健康检查
    
    // 检查 Processor 状态
    processorHealth := h.checkProcessorHealth()
    
    systemHealth.Components = append(systemHealth.Components, processorHealth)
    
    // ...
}

func (h *HealthHandler) checkProcessorHealth() ComponentHealth {
    resp, err := http.Get("http://processor-jobmanager:8081/overview")
    if err != nil {
        return ComponentHealth{
            Name:    "processor",
            Healthy: false,
            Status:  "unreachable",
            Error:   err.Error(),
        }
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != 200 {
        return ComponentHealth{
            Name:    "processor",
            Healthy: false,
            Status:  "unhealthy",
            Error:   fmt.Sprintf("HTTP %d", resp.StatusCode),
        }
    }
    
    return ComponentHealth{
        Name:         "processor",
        Healthy:      true,
        Status:       "healthy",
        ResponseTime: "< 100ms",
    }
}
```

## 📈 扩展功能

### 1. 作业管理
- 提交新作业
- 停止/重启作业
- 作业配置更新

### 2. 规则管理
- 威胁检测规则查看
- 规则配置更新
- 规则效果统计

### 3. 性能优化
- 并行度调整建议
- 资源配置优化
- 性能瓶颈分析

---

**集成优势**:
- ✅ **实时监控**: 获取 Processor 实时状态和指标
- ✅ **统一管理**: 通过 Manager 统一管理所有组件
- ✅ **故障诊断**: 快速定位 Processor 相关问题
- ✅ **性能优化**: 基于指标数据进行性能调优
- ✅ **运维友好**: 简化 Processor 运维操作
