# Nova 分支集成计划

## 🎯 集成目标

将 nova 分支的双向心跳机制集成到当前 Monorepo 架构中，实现 Collector 主动上报 + Manager 主动探测的完整监控体系。

## 📊 Nova 分支核心功能分析

### 双向心跳机制
```
方向1: Collector → Manager (主动上报)
- Collector 每分钟发送心跳到 Manager
- 包含系统状态检查 (rsyslog, auditd, 配置文件)
- 支持重试机制和错误处理

方向2: Manager → Collector (主动探测)  
- Manager 通过 UDP syslog 发送探测消息
- Collector 通过 rsyslog omprog 模块响应
- 实时验证 Collector 响应能力
```

### 数据库增强
```sql
-- 新增字段
ALTER TABLE collectors ADD COLUMN last_active TIMESTAMP;

-- 新增索引
CREATE INDEX idx_collectors_last_active ON collectors(last_active);
CREATE INDEX idx_collectors_status_last_active ON collectors(status, last_active);

-- 状态逻辑
last_heartbeat: 最后收到心跳时间 (被动接收)
last_active: 最后确认活跃时间 (主动确认)
```

## 🔧 集成实施计划

### Phase 1: 数据库模式迁移

#### 1.1 迁移文件创建
```bash
# 目标文件: sysarmor/shared/migrations/002_add_last_active.sql
```

#### 1.2 Collector 模型扩展
```go
// 目标文件: sysarmor/apps/manager/models/collector.go
type Collector struct {
    // 现有字段...
    LastActive    *time.Time `json:"last_active,omitempty" db:"last_active"`     // 新增
}

// 目标文件: sysarmor/apps/manager/models/request.go  
type CollectorStatus struct {
    // 现有字段...
    LastActive    *time.Time `json:"last_active,omitempty"`                      // 新增
}

// 新增心跳相关模型
type HeartbeatRequest struct {
    Status  string `json:"status" binding:"required,oneof=active inactive error offline unregistered"`
    ProbeID string `json:"probe_id,omitempty"`  // 可选，用于探测响应
}

type HeartbeatResponse struct {
    Success               bool      `json:"success"`
    NextHeartbeatInterval int       `json:"next_heartbeat_interval"`
    ServerTime            time.Time `json:"server_time"`
}

type ProbeResponse struct {
    CollectorID     string     `json:"collector_id"`
    Success         bool       `json:"success"`
    ProbeID         string     `json:"probe_id"`
    SentAt          time.Time  `json:"sent_at"`
    HeartbeatBefore *time.Time `json:"heartbeat_before,omitempty"`
    HeartbeatAfter  *time.Time `json:"heartbeat_after,omitempty"`
    ErrorMessage    string     `json:"error_message,omitempty"`
}
```

#### 1.3 常量定义扩展
```go
// 目标文件: sysarmor/apps/manager/models/constants.go
const (
    // 现有状态...
    CollectorStatusOffline = "offline"  // 新增: 长时间无心跳
)
```

### Phase 2: 数据库操作层扩展

#### 2.1 Repository 方法扩展
```go
// 目标文件: sysarmor/apps/manager/storage/repository.go

// 新增方法: 心跳状态更新
func (r *Repository) UpdateHeartbeatWithStatus(ctx context.Context, collectorID string, status string) error {
    now := time.Now()
    
    if status == "active" {
        // 活跃状态: 同时更新 last_heartbeat 和 last_active
        query := `UPDATE collectors SET last_heartbeat = $1, last_active = $1, status = $2, updated_at = $1 WHERE collector_id = $3`
        _, err := r.db.ExecContext(ctx, query, now, status, collectorID)
        return err
    } else {
        // 非活跃状态: 只更新 last_heartbeat，保持 last_active 不变
        query := `UPDATE collectors SET last_heartbeat = $1, status = $2, updated_at = $1 WHERE collector_id = $3`
        _, err := r.db.ExecContext(ctx, query, now, status, collectorID)
        return err
    }
}

// 修改现有方法: 包含 last_active 字段
func (r *Repository) GetByID(ctx context.Context, collectorID string) (*models.Collector, error) {
    query := `SELECT collector_id, hostname, ip_address, os_type, os_version, deployment_type, 
              status, worker_address, kafka_topic, last_heartbeat, last_active, created_at, updated_at 
              FROM collectors WHERE collector_id = $1`
    // ...
}

func (r *Repository) List(ctx context.Context, filters *models.CollectorFilters, pagination *models.PaginationRequest, sort *models.SortRequest) ([]*models.Collector, int, error) {
    // 更新查询语句包含 last_active 字段
    // ...
}
```

### Phase 3: API 处理器扩展

#### 3.1 心跳 API 实现
```go
// 目标文件: sysarmor/apps/manager/api/handlers/collector.go

// 新增方法: 心跳上报处理
// @Summary 接收 Collector 心跳
// @Description Collector 主动上报心跳状态
// @Tags collectors
// @Accept json
// @Produce json
// @Param id path string true "Collector ID"
// @Param request body models.HeartbeatRequest true "心跳请求"
// @Success 200 {object} models.HeartbeatResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /collectors/{id}/heartbeat [post]
func (h *CollectorHandler) Heartbeat(c *gin.Context) {
    collectorID := c.Param("id")
    
    var req models.HeartbeatRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "error":   "Invalid request format: " + err.Error(),
        })
        return
    }
    
    ctx := c.Request.Context()
    
    // 验证 Collector 是否存在
    _, err := h.repo.GetByID(ctx, collectorID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{
            "success": false,
            "error":   "Collector not found",
        })
        return
    }
    
    // 更新心跳状态
    err = h.repo.UpdateHeartbeatWithStatus(ctx, collectorID, req.Status)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Failed to update heartbeat: " + err.Error(),
        })
        return
    }
    
    // 返回响应
    response := models.HeartbeatResponse{
        Success:               true,
        NextHeartbeatInterval: 60, // 60秒间隔
        ServerTime:            time.Now(),
    }
    
    c.JSON(http.StatusOK, response)
}

// 新增方法: 主动探测处理
// @Summary 主动探测 Collector
// @Description Manager 主动探测 Collector 响应能力
// @Tags collectors
// @Accept json
// @Produce json
// @Param id path string true "Collector ID"
// @Param timeout query int false "探测超时时间(秒)" default(10)
// @Success 200 {object} models.ProbeResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /collectors/{id}/heartbeat [get]
func (h *CollectorHandler) ProbeHeartbeat(c *gin.Context) {
    collectorID := c.Param("id")
    
    // 解析超时参数
    timeout := 10
    if timeoutStr := c.Query("timeout"); timeoutStr != "" {
        if t, err := strconv.Atoi(timeoutStr); err == nil && t > 0 && t <= 60 {
            timeout = t
        }
    }
    
    ctx := c.Request.Context()
    
    // 获取 Collector 信息
    collector, err := h.repo.GetByID(ctx, collectorID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{
            "success": false,
            "error":   "Collector not found",
        })
        return
    }
    
    // 执行探测
    probeResponse, err := h.sendProbeRequest(ctx, collector, timeout)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error":   "Probe failed: " + err.Error(),
        })
        return
    }
    
    c.JSON(http.StatusOK, probeResponse)
}

// 探测实现方法
func (h *CollectorHandler) sendProbeRequest(ctx context.Context, collector *models.Collector, timeoutSeconds int) (*models.ProbeResponse, error) {
    // 实现探测逻辑 (参考 API_OPTIMIZATION_ANALYSIS.md 中的详细实现)
    // ...
}
```

### Phase 4: 模板系统增强

#### 4.1 Agentless 模板更新
```bash
# 目标文件: sysarmor/shared/templates/agentless/setup-terminal.sh.tmpl
# 需要添加的内容 (+228行):

# 1. 心跳脚本创建
cat > /usr/local/bin/sysarmor-heartbeat.sh << 'HEARTBEAT_SCRIPT_EOF'
#!/bin/bash
COLLECTOR_ID="{{.CollectorID}}"
MANAGER_URL="{{.ManagerURL}}"
LOG_FILE="/var/log/sysarmor/heartbeat.log"
LOCK_FILE="/var/run/sysarmor-heartbeat.lock"
# ... (174行心跳脚本逻辑)
HEARTBEAT_SCRIPT_EOF

# 2. 设置权限和定时任务
chmod +x /usr/local/bin/sysarmor-heartbeat.sh
(crontab -l 2>/dev/null; echo "*/1 * * * * /usr/local/bin/sysarmor-heartbeat.sh >/dev/null 2>&1") | crontab -

# 3. rsyslog 配置增强
# 添加 UDP 接收和 omprog 模块配置
module(load="imudp")
module(load="omprog")
input(type="imudp" port="514")

if \$msg contains "SYSARMOR_PROBE:" then {
    action(type="omprog" binary="/usr/local/bin/sysarmor-heartbeat.sh")
    stop
}
```

#### 4.2 卸载模板更新
```bash
# 目标文件: sysarmor/shared/templates/agentless/uninstall-terminal.sh.tmpl
# 需要添加的清理逻辑 (+50行):

# 1. 停止和删除定时任务
crontab -l | grep -v "sysarmor-heartbeat" | crontab -

# 2. 删除心跳脚本
rm -f /usr/local/bin/sysarmor-heartbeat.sh

# 3. 清理日志文件
rm -f /var/log/sysarmor/heartbeat.log
rm -f /var/log/sysarmor/probe.log

# 4. 清理锁文件
rm -f /var/run/sysarmor-heartbeat.lock
```

### Phase 5: 配置系统扩展

#### 5.1 环境变量支持
```go
// 目标文件: sysarmor/apps/manager/config/config.go

// 新增 dotenv 支持
import "github.com/joho/godotenv"

func Load() (*Config, error) {
    // 加载 .env 文件
    envFile := os.Getenv("ENV_FILE")
    if envFile == "" {
        envFile = ".env"
    }
    
    if _, err := os.Stat(envFile); err == nil {
        if err := godotenv.Load(envFile); err == nil {
            fmt.Printf("✅ Loaded environment from: %s\n", envFile)
        }
    }
    
    // 现有配置加载逻辑...
}
```

### Phase 6: API 路由更新

#### 6.1 心跳和探测路由设计

```go
// 目标文件: sysarmor/apps/manager/main.go

collectors := api.Group("/collectors")
{
    // 现有路由...
    collectors.GET("/:id", collectorHandler.GetStatus)                 // 获取状态 (从数据库)
    
    // 心跳相关路由
    collectors.POST("/:id/heartbeat", collectorHandler.Heartbeat)      // 心跳上报
    collectors.POST("/:id/probe", collectorHandler.ProbeHeartbeat)     // 主动探测 (改为POST)
    
    // 批量状态查询
    collectors.GET("", collectorHandler.ListCollectors)               // 批量状态 (从数据库)
}
```

#### 6.2 API 职责分离

```go
// 1. 状态查询 API (从数据库读取，快速响应)
GET /api/v1/collectors/{id}           // 单个 Collector 状态
GET /api/v1/collectors                // 批量 Collector 状态
// 特点: 
// - 从数据库直接读取 last_heartbeat, last_active 等字段
// - 响应速度快 (~5ms)
// - 适合前端频繁查询
// - 显示历史状态信息

// 2. 心跳上报 API (Collector 主动调用)
POST /api/v1/collectors/{id}/heartbeat
// 特点:
// - Collector 定时调用 (每分钟)
// - 更新数据库状态
// - 包含系统健康检查结果

// 3. 主动探测 API (管理员手动触发)
POST /api/v1/collectors/{id}/probe?timeout=10
// 特点:
// - 管理员手动触发的实时探测
// - 发送 UDP 消息并等待响应
// - 响应时间较长 (最多 timeout 秒)
// - 用于故障诊断和连接测试
```

## 🔍 API 设计说明

### 关键问题: 状态查询 vs 主动探测

#### 问题分析
```
前端需求: 
- 快速获取 Collector 在线状态 (用于仪表板显示)
- 频繁查询 (每几秒刷新一次)
- 批量查询多个 Collector 状态

原设计问题:
GET /api/v1/collectors/{id}/heartbeat  # 直接触发探测，响应慢 (最多60秒)
- 不适合前端频繁调用
- 会产生大量网络探测请求
- 影响系统性能
```

#### 优化后的API设计

```go
// 1. 快速状态查询 (前端使用) - 从数据库读取
GET /api/v1/collectors/{id}                    // 单个状态查询 (~5ms)
GET /api/v1/collectors                         // 批量状态查询 (~20ms)
// 返回: last_heartbeat, last_active, status 等数据库字段
// 适合: 前端仪表板、状态监控、批量查询

// 2. 心跳上报 (Collector 使用) - 更新数据库
POST /api/v1/collectors/{id}/heartbeat         // Collector 主动上报
// 用途: Collector 定时发送心跳 (每分钟)
// 效果: 更新数据库中的 last_heartbeat, last_active, status

// 3. 主动探测 (管理员使用) - 实时网络测试
POST /api/v1/collectors/{id}/probe?timeout=10  // 管理员手动触发
// 用途: 故障诊断、连接测试、网络质量检查
// 特点: 响应时间长 (最多 timeout 秒)，不适合频繁调用
```

#### 前端使用场景

```javascript
// 前端仪表板 - 快速状态查询
async function getCollectorStatus(collectorId) {
    const response = await fetch(`/api/v1/collectors/${collectorId}`);
    const data = await response.json();
    
    // 基于数据库字段判断状态
    const now = new Date();
    const lastActive = new Date(data.last_active);
    const timeDiff = (now - lastActive) / 1000 / 60; // 分钟
    
    if (timeDiff <= 5) return 'online';      // 5分钟内活跃
    if (timeDiff <= 30) return 'inactive';   // 30分钟内有心跳但不活跃
    return 'offline';                        // 长时间无响应
}

// 批量状态查询
async function getAllCollectorStatus() {
    const response = await fetch('/api/v1/collectors');
    return response.json(); // 返回所有 Collector 状态
}

// 管理员诊断 - 主动探测 (谨慎使用)
async function probeCollector(collectorId) {
    const response = await fetch(`/api/v1/collectors/${collectorId}/probe`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ timeout: 10 })
    });
    return response.json(); // 返回探测结果
}
```

#### 状态判断逻辑

```go
// 在 Collector 模型中添加状态判断方法
func (c *Collector) GetRealTimeStatus() string {
    now := time.Now()
    
    // 1. 检查最近心跳 (5分钟内)
    if c.LastHeartbeat != nil && now.Sub(*c.LastHeartbeat) <= 5*time.Minute {
        return c.Status  // 返回 Collector 上报的状态 (active/inactive/error)
    }
    
    // 2. 检查最近活跃 (30分钟内)
    if c.LastActive != nil && now.Sub(*c.LastActive) <= 30*time.Minute {
        return "inactive"  // 最近活跃过但现在无心跳
    }
    
    // 3. 长时间无响应
    return "offline"  // 长时间无任何响应
}

// 在 API 响应中包含计算后的状态
type CollectorStatusResponse struct {
    *models.Collector
    RealTimeStatus string `json:"realtime_status"`  // 计算后的实时状态
    LastSeenMinutes int   `json:"last_seen_minutes"` // 最后活跃分钟数
}
```

## 🧪 集成测试计划

### 测试用例设计
```bash
# 1. 状态查询测试 (前端场景)
curl "http://localhost:8080/api/v1/collectors/{id}"
# 预期: 快速返回 (~5ms)，包含 last_active, realtime_status 字段

curl "http://localhost:8080/api/v1/collectors"
# 预期: 批量返回所有 Collector 状态

# 2. 心跳上报测试 (Collector 场景)
curl -X POST http://localhost:8080/api/v1/collectors/{id}/heartbeat \
  -d '{"status":"active"}'
# 预期: 200 OK, 更新数据库字段

# 3. 主动探测测试 (管理员场景)
curl -X POST http://localhost:8080/api/v1/collectors/{id}/probe \
  -d '{"timeout":10}'
# 预期: 发送 UDP 探测，返回详细探测结果
```

### 集成测试脚本
```bash
# 目标文件: sysarmor/tests/migrations/test-nova.sh

#!/bin/bash
# Nova 分支双向心跳功能测试

# 1. 注册测试 Collector
# 2. 测试心跳上报 API
# 3. 测试主动探测 API  
# 4. 验证数据库字段更新
# 5. 测试增强的安装脚本
```

## 📋 实施步骤

### Step 1: 数据库迁移 (优先级: 高)
- [ ] 创建 `002_add_last_active.sql` 迁移文件
- [ ] 更新 Collector 模型添加 `LastActive` 字段
- [ ] 更新所有查询语句包含新字段
- [ ] 执行数据库迁移

### Step 2: API 实现 (优先级: 高)
- [ ] 实现 `Heartbeat()` 方法 (心跳上报)
- [ ] 实现 `ProbeHeartbeat()` 方法 (主动探测)
- [ ] 添加心跳相关模型定义
- [ ] 更新 API 路由配置

### Step 3: 探测机制实现 (优先级: 中)
- [ ] 实现 UDP syslog 消息发送
- [ ] 实现探测响应轮询逻辑
- [ ] 添加网络错误处理
- [ ] 实现探测结果返回

### Step 4: 模板系统增强 (优先级: 中)
- [ ] 更新 agentless 安装模板 (+228行)
- [ ] 添加心跳脚本生成逻辑
- [ ] 添加 rsyslog 配置增强
- [ ] 更新卸载模板清理逻辑

### Step 5: 配置系统扩展 (优先级: 低)
- [ ] 添加 dotenv 支持
- [ ] 更新配置加载逻辑
- [ ] 添加环境变量文档

### Step 6: 测试和验证 (优先级: 高)
- [ ] 创建集成测试脚本
- [ ] 验证双向心跳功能
- [ ] 测试数据库字段更新
- [ ] 验证增强的安装脚本

## 🔄 迁移策略

### 向后兼容性
```go
// 数据库迁移策略
UPDATE collectors SET last_active = updated_at WHERE last_active IS NULL;
// 确保现有 Collector 有合理的 last_active 值

// API 兼容性
// 现有的 Collector 查询 API 保持不变，只是响应中多了 last_active 字段
```

### 渐进式部署
```bash
# 阶段1: 数据库和 API 就绪
- 部署新的 Manager 版本
- 现有 Collector 继续工作 (无心跳功能)

# 阶段2: 新 Collector 启用心跳
- 新注册的 Collector 使用增强的安装脚本
- 自动启用双向心跳功能

# 阶段3: 现有 Collector 升级
- 提供升级脚本为现有 Collector 添加心跳功能
- 逐步迁移到新的监控机制
```

## 🎯 预期收益

### 监控能力提升
- **精确状态跟踪**: 通过 last_active 字段精确了解 Collector 活跃状态
- **主动故障检测**: Manager 可以主动验证 Collector 响应能力
- **网络质量监控**: 通过探测延迟了解网络连接质量

### 运维效率提升
- **实时告警**: 基于 last_active 实现精确的离线告警
- **故障诊断**: 详细的探测结果帮助快速定位问题
- **自动化监控**: 减少人工检查，提高运维效率

## 🔮 风险评估

### 技术风险 (低)
- **数据库迁移**: 风险低，只是添加字段和索引
- **API 扩展**: 风险低，不影响现有功能
- **网络依赖**: 需要确保 UDP 514 端口可达

### 兼容性风险 (极低)
- **现有 Collector**: 完全兼容，无需立即升级
- **API 接口**: 向后兼容，只是响应中多了字段
- **数据库**: 新字段允许 NULL，不影响现有数据

## 📅 实施时间线

### 第1周: 核心功能实现
- Day 1-2: 数据库迁移和模型更新
- Day 3-4: API 实现和测试
- Day 5: 探测机制实现

### 第2周: 模板和测试
- Day 1-3: 模板系统增强
- Day 4-5: 集成测试和验证

### 第3周: 部署和优化
- Day 1-2: 生产环境部署
- Day 3-5: 监控和优化

---

**Nova 分支集成总结**: 通过双向心跳机制，SysArmor 将获得更精确的 Collector 监控能力，实现主动探测和被动接收的完美结合，大幅提升系统的可观测性和故障诊断能力。
