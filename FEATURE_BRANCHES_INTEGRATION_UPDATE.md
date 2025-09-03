# SysArmor 功能分支集成更新方案

## 🎯 基于 Patch 分析的完整集成方案

通过对三个功能分支的 patch 文件深入分析，现提供详细的 Monorepo 集成方案。

## 📊 分支改动详细分析

### 1. **dev-zheng 分支** - OpenTelemetry Collector 二进制分发

#### 🔍 **核心改动统计**
- **新增文件**: 9个
- **修改文件**: 6个  
- **新增代码**: 800+ 行
- **主要功能**: 二进制文件分发 + OpenTelemetry Collector 集成

#### 📋 **详细改动内容**

##### **新增文件**
```bash
# 二进制文件 (Git LFS)
data/dist/otelcol-sysarmor_linux-x64          # 34MB OpenTelemetry Collector 二进制

# OpenTelemetry 模板
templates/collector-otel/cfg.yaml.tmpl        # OTel 配置模板
templates/collector-otel/install-otelcol.sh.tmpl  # 安装脚本模板 (311行)
templates/collector-otel/install-sysdig.sh    # Sysdig 安装脚本 (152行)

# API 处理器
internal/api/handlers/download.go             # 文件下载处理器 (131行)

# 文档
CHANGELOG.md                                  # 变更日志
CLAUDE.md                                     # Claude AI 指导文档
```

##### **核心功能实现**
```go
// 1. 二进制文件下载 API
GET /api/v1/download/:filename

// 2. OpenTelemetry Collector 配置
receivers:
  sysdig:
    command: ["sysdig", "-p", "*{...}", "proc.name!=sysdig and (fd.name exists and fd.name != \"\")"]
    subject: "events.sysdig.{{.CollectorID}}"

exporters:
  sysarmormiddleware:
    host: "{{.WorkerHost}}"
    port: {{.WorkerPort}}
    batch_size: 1000
    max_retries: 3

// 3. 安全的文件下载
func (h *DownloadHandler) DownloadFile(c *gin.Context) {
    // 路径遍历攻击防护
    // 文件存在性验证
    // 安全的文件传输
}
```

---

### 2. **nova 分支** - 双向心跳和探测系统

#### 🔍 **核心改动统计**
- **修改文件**: 15个
- **新增代码**: 1056+ 行
- **主要功能**: 双向心跳机制 + 主动探测 + 数据库增强

#### 📋 **详细改动内容**

##### **数据库模式增强**
```sql
-- 新增字段和索引
ALTER TABLE collectors ADD COLUMN IF NOT EXISTS last_active TIMESTAMP;
CREATE INDEX IF NOT EXISTS idx_collectors_last_active ON collectors(last_active);
CREATE INDEX IF NOT EXISTS idx_collectors_status_last_active ON collectors(status, last_active);

-- 更新现有记录
UPDATE collectors SET last_active = updated_at WHERE last_active IS NULL;
```

##### **心跳和探测模型**
```go
// 心跳请求模型
type HeartbeatRequest struct {
    Status string `json:"status"` // collector状态
}

// 心跳响应模型
type HeartbeatResponse struct {
    Success               bool      `json:"success"`
    NextHeartbeatInterval int       `json:"next_heartbeat_interval"`
    ServerTime            time.Time `json:"server_time"`
}

// 探测响应模型
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

##### **双向心跳机制**
```go
// POST /collectors/:id/heartbeat - Collector 主动上报
func (h *CollectorHandler) Heartbeat(c *gin.Context) {
    // 1. 解析心跳请求
    // 2. 验证状态值
    // 3. 更新数据库 (last_heartbeat + last_active + status)
    // 4. 返回下次心跳间隔
}

// GET /collectors/:id/heartbeat - Manager 主动探测
func (h *CollectorHandler) ProbeHeartbeat(c *gin.Context) {
    // 1. 生成唯一 probe ID
    // 2. 发送 UDP syslog 消息到 collector:514
    // 3. 轮询检查心跳是否更新
    // 4. 返回探测结果
}
```

##### **安装脚本大幅增强** (+228行)
```bash
# 新增心跳脚本 (174行)
/usr/local/bin/sysarmor-heartbeat.sh:
- 系统状态检查 (rsyslog, auditd, 配置文件)
- HTTP 心跳上报到 Manager
- 重试机制和错误处理
- 锁文件防止重复执行

# 新增 probe 处理脚本
/usr/local/bin/sysarmor-probe-handler.sh:
- 处理 Manager 发来的 probe 消息
- 提取 probe_id 并响应
- 通过 rsyslog omprog 模块触发

# Crontab 定时任务
*/1 * * * * /usr/local/bin/sysarmor-heartbeat.sh  # 每分钟执行

# Rsyslog 配置增强
module(load="imudp")     # UDP接收模块
module(load="omprog")    # 程序执行模块
input(type="imudp" port="514")  # 监听 probe 消息

# Probe 消息处理
if $msg contains "SYSARMOR_PROBE:" then {
    action(type="omprog" binary="/usr/local/bin/sysarmor-heartbeat.sh")
    stop
}
```

##### **dotenv 配置支持**
```go
// 新增环境变量文件加载
func Load() *Config {
    envFile := os.Getenv("ENV_FILE")
    if envFile != "" {
        if err := godotenv.Load(envFile); err == nil {
            fmt.Printf("Loaded environment from: %s\n", envFile)
        }
    }
    // ...
}
```

---

### 3. **hfw 分支** - 完整 Wazuh 生态系统集成

#### 🔍 **核心改动统计**
- **新增文件**: 8个
- **修改文件**: 7个
- **新增代码**: 4000+ 行
- **主要功能**: Wazuh Manager + Indexer 完整集成

#### 📋 **详细改动内容**

##### **完整的 Wazuh 数据模型** (1491行)
```go
// 核心模型
type WazuhAgent struct {
    ID                string       `json:"id"`
    Name              string       `json:"name"`
    IP                string       `json:"ip"`
    Status            string       `json:"status"`
    LastKeepAlive     *time.Time   `json:"last_keepalive,omitempty"`
    OS                *WazuhOSInfo `json:"os,omitempty"`
    // ... 20+ 字段
}

// 系统信息模型
type WazuhHardwareInfo struct {
    CPU         *WazuhCPUInfo `json:"cpu,omitempty"`
    RAM         *WazuhRAMInfo `json:"ram,omitempty"`
    BoardSerial string        `json:"board_serial,omitempty"`
}

// 事件搜索模型
type WazuhSearchQuery struct {
    IndexType string      `json:"index_type,omitempty"` // alerts or archives
    Query     interface{} `json:"query,omitempty"`      // 支持字符串或复杂DSL
    StartTime time.Time   `json:"start_time,omitempty"`
    EndTime   time.Time   `json:"end_time,omitempty"`
    // ... 更多字段
}
```

##### **完整的 Wazuh API 集成** (2291行)
```go
// Manager API 端点 (30+ 个)
GET  /api/v1/wazuh/agents                    # 代理列表
POST /api/v1/wazuh/agents                    # 添加代理
GET  /api/v1/wazuh/agents/:id                # 代理详情
DELETE /api/v1/wazuh/agents/:id              # 删除代理
PUT  /api/v1/wazuh/agents/:id/restart        # 重启代理
GET  /api/v1/wazuh/agents/:id/key            # 代理密钥
GET  /api/v1/wazuh/agents/:id/hardware       # 硬件信息
GET  /api/v1/wazuh/agents/:id/processes      # 进程信息
GET  /api/v1/wazuh/agents/:id/packages       # 软件包信息
GET  /api/v1/wazuh/agents/:id/ports          # 端口信息
PUT  /api/v1/wazuh/agents/:id/active-response # 主动响应

// Indexer API 端点
POST /api/v1/wazuh/events/search             # 事件搜索
GET  /api/v1/wazuh/events/:index_type/:id    # 单个事件
POST /api/v1/wazuh/events/aggregations       # 聚合统计
GET  /api/v1/wazuh/indices                   # 索引列表
GET  /api/v1/wazuh/cluster/health            # 集群健康

// 配置管理 API
PUT  /api/v1/wazuh/config/auth               # 更新认证配置
GET  /api/v1/wazuh/config/auth               # 获取认证配置
POST /api/v1/wazuh/config/test               # 测试连接
```

##### **动态配置管理** (674行)
```go
// 配置管理器
type ConfigManager struct {
    staticConfig   *config.WazuhConfig
    dynamicConfig  *models.WazuhDynamicAuthRequest
    managerClient  *ManagerClient
    indexerClient  *IndexerClient
    configFilePath string
}

// 动态配置更新
func (cm *ConfigManager) UpdateConfig(ctx context.Context, req *models.WazuhDynamicAuthRequest) error {
    // 1. 验证配置
    // 2. 创建备份
    // 3. 测试连接
    // 4. 更新 YAML 配置文件
    // 5. 重新创建客户端
}
```

##### **Wazuh 配置文件**
```yaml
# configs/wazuh.yaml
wazuh:
  manager:
    url: "https://10.129.81.4:55000"
    username: "wazuh"
    password: "WfvmoiqFu*0g0t425lj*Y.3SBZOYmUCR"
    timeout: 30s
    token_expiry: 7200s
    tls_verify: false
    max_retries: 3
  
  indexer:
    url: "https://10.129.81.4:9200"
    username: "admin"
    password: "5uSbeSyPANO?8rcbgvAF8frpANOWon+D"
    timeout: 30s
    tls_verify: false
    max_retries: 3
    indices:
      alerts: "wazuh-alerts-*"
      archives: "wazuh-archives-*"
  
  features:
    manager_enabled: true
    indexer_enabled: true
    auto_sync: true
    sync_interval: 300s
```

## 🚀 Monorepo 集成实施方案

### **Phase 1: 数据库和模型层集成** (优先级: 高)

#### 1.1 数据库迁移
```bash
# 复制迁移文件
cp scripts/sysarmor-manager/migrations/002_add_last_active.sql sysarmor/shared/migrations/

# 执行迁移
docker compose exec postgres psql -U sysarmor -d sysarmor -f /docker-entrypoint-initdb.d/002_add_last_active.sql
```

#### 1.2 模型层集成
```bash
# 目标: sysarmor/apps/manager/models/
├── collector.go     # 现有 + LastActive 字段 + 心跳模型
├── constants.go     # 现有 + CollectorStatusOffline 状态
├── request.go       # 现有 + LastActive 字段
├── utils.go         # 新增 (来自 nova 分支)
├── error.go         # 新增 (来自 hfw 分支)
└── wazuh.go         # 新增 (来自 hfw 分支, 1491行)
```

#### 1.3 配置层集成
```bash
# 目标: sysarmor/apps/manager/config/
├── config.go        # 现有 + DownloadDir + WazuhConfigPath + dotenv 支持
└── wazuh_config.go  # 新增 (来自 hfw 分支, 166行)

# 目标: sysarmor/shared/
├── templates/
│   ├── agentless/   # 现有 + 心跳功能增强
│   ├── collector-otel/  # 新增 (来自 dev-zheng 分支)
│   └── wazuh/       # 新增 (预留)
└── configs/
    └── wazuh.yaml   # 新增 Wazuh 配置文件
```

### **Phase 2: 服务层集成** (优先级: 中)

#### 2.1 服务层扩展
```bash
# 目标: sysarmor/apps/manager/services/
├── template/        # 现有 + ExtraCfgData 支持 + ManagerURL
├── download/        # 新增 (二进制文件下载服务)
└── wazuh/          # 新增 (来自 hfw 分支)
    ├── config_manager.go    # 配置管理器 (674行)
    ├── manager_client.go    # Manager 客户端 (1459行)
    └── indexer_client.go    # Indexer 客户端 (591行)
```

#### 2.2 API 处理器集成
```bash
# 目标: sysarmor/apps/manager/api/handlers/
├── collector.go     # 现有 + 心跳和探测功能 (+226行)
├── download.go      # 新增 (来自 dev-zheng 分支, 131行)
└── wazuh.go         # 新增 (来自 hfw 分支, 2291行)
```

### **Phase 3: 路由和 API 集成** (优先级: 中)

#### 3.1 新增 API 路由
```go
// 心跳和探测 API (nova 分支)
collectors.POST("/:id/heartbeat", collectorHandler.Heartbeat)      // 心跳上报
collectors.GET("/:id/heartbeat", collectorHandler.ProbeHeartbeat)  // 主动探测

// 二进制下载 API (dev-zheng 分支)
download.GET("/:filename", downloadHandler.DownloadFile)

// 脚本下载增强 (dev-zheng 分支)
scripts.GET("/agentless/setup-terminal.sh", collectorHandler.DownloadScript)
scripts.GET("/sysarmor-stack/install-collector.sh", collectorHandler.DownloadScript)
scripts.GET("/wazuh-hybrid/install-wazuh.sh", collectorHandler.DownloadScript)
scripts.GET("/otelcol/install.sh", collectorHandler.DownloadScript)

// Wazuh 管理 API (hfw 分支) - 30+ 个端点
wazuh := services.Group("/wazuh")
{
    // Manager 管理
    wazuh.GET("/manager/info", wazuhHandler.GetManagerInfo)
    wazuh.GET("/manager/status", wazuhHandler.GetManagerStatus)
    
    // 代理管理
    wazuh.GET("/agents", wazuhHandler.GetAgents)
    wazuh.POST("/agents", wazuhHandler.AddAgent)
    wazuh.DELETE("/agents/:id", wazuhHandler.DeleteAgent)
    wazuh.PUT("/agents/:id/restart", wazuhHandler.RestartAgent)
    
    // 系统信息
    wazuh.GET("/agents/:id/hardware", wazuhHandler.GetHardwareInfo)
    wazuh.GET("/agents/:id/processes", wazuhHandler.GetProcesses)
    wazuh.GET("/agents/:id/packages", wazuhHandler.GetPackages)
    wazuh.GET("/agents/:id/ports", wazuhHandler.GetPorts)
    
    // 事件搜索
    wazuh.POST("/events/search", wazuhHandler.SearchEvents)
    wazuh.GET("/events/:index_type/:id", wazuhHandler.GetEventByID)
    
    // 配置管理
    wazuh.PUT("/config/auth", wazuhHandler.UpdateWazuhAuth)
    wazuh.GET("/config/auth", wazuhHandler.GetWazuhAuth)
    wazuh.POST("/config/test", wazuhHandler.TestWazuhConnection)
}
```

### **Phase 4: 部署类型扩展**

#### 4.1 新增部署类型常量
```go
const (
    DeploymentTypeAgentless     = "agentless"       // 现有
    DeploymentTypeSysArmor      = "sysarmor-stack"  // 现有
    DeploymentTypeWazuh         = "wazuh-hybrid"    // 现有
    DeploymentTypeOTelCollector = "otel-collector"  // 新增
)
```

#### 4.2 模板系统扩展
```bash
shared/templates/
├── agentless/           # 现有 + 心跳功能增强
│   ├── setup-terminal.sh.tmpl    # +228行心跳功能
│   ├── uninstall-terminal.sh.tmpl # +50行心跳清理
│   └── audit-rules.tmpl          # 现有
├── collector-otel/      # 新增 (来自 dev-zheng 分支)
│   ├── cfg.yaml.tmpl             # OpenTelemetry 配置
│   ├── install-otelcol.sh.tmpl   # 安装脚本 (311行)
│   └── install-sysdig.sh         # Sysdig 安装脚本 (152行)
└── wazuh/              # 新增 (预留)
    ├── agent-install.sh.tmpl
    └── ossec.conf.tmpl
```

## 🛠️ 具体集成步骤

### **Step 1: 立即集成 - nova 分支 (健康监测)**

```bash
# 1. 数据库迁移
cp scripts/sysarmor-manager/migrations/002_add_last_active.sql sysarmor/shared/migrations/

# 2. 模型更新
# - 在 sysarmor/apps/manager/models/collector.go 中添加 LastActive 字段
# - 添加心跳和探测相关模型
# - 新增 CollectorStatusOffline 状态

# 3. API 处理器更新
# - 在 collector.go 中添加 Heartbeat 和 ProbeHeartbeat 方法
# - 更新所有返回 CollectorStatus 的地方，包含 LastActive 字段

# 4. 数据库操作更新
# - 添加 UpdateHeartbeatWithStatus 方法
# - 更新所有查询语句包含 last_active 字段

# 5. 模板更新
# - 更新 agentless 模板，添加心跳脚本和 probe 处理
# - 添加 ManagerURL 模板变量支持
```

### **Step 2: 短期集成 - dev-zheng 分支 (OTel Collector)**

```bash
# 1. 二进制文件管理
mkdir -p sysarmor/data/dist
# 注意: 需要使用 Git LFS 管理大文件

# 2. 下载 API 集成
cp scripts/sysarmor-manager/internal/api/handlers/download.go sysarmor/apps/manager/api/handlers/

# 3. 模板系统扩展
cp -r scripts/sysarmor-manager/templates/collector-otel sysarmor/shared/templates/

# 4. 配置更新
# - 在 config.go 中添加 DownloadDir 配置
# - 更新 Docker 配置支持文件挂载

# 5. 路由更新
# - 添加 /api/v1/download/:filename 路由
# - 扩展脚本下载路由支持多种部署类型
```

### **Step 3: 中期集成 - hfw 分支 (Wazuh 支持)**

```bash
# 1. Wazuh 模型集成
cp scripts/sysarmor-manager/internal/models/wazuh.go sysarmor/apps/manager/models/
cp scripts/sysarmor-manager/internal/models/error.go sysarmor/apps/manager/models/

# 2. Wazuh 配置集成
cp scripts/sysarmor-manager/internal/config/wazuh_config.go sysarmor/apps/manager/config/
cp scripts/sysarmor-manager/configs/wazuh.yaml sysarmor/shared/configs/

# 3. Wazuh 服务集成
mkdir -p sysarmor/apps/manager/services/wazuh
cp -r scripts/sysarmor-manager/internal/services/wazuh/* sysarmor/apps/manager/services/wazuh/

# 4. Wazuh API 处理器集成
cp scripts/sysarmor-manager/internal/api/handlers/wazuh.go sysarmor/apps/manager/api/handlers/

# 5. 主路由更新
# - 添加完整的 Wazuh API 路由组
# - 集成配置管理和健康检查
```

## 📊 集成后的功能增强

### **新增 API 端点总览**
```
# 心跳和探测 (nova)
POST /api/v1/collectors/:id/heartbeat        # 心跳上报
GET  /api/v1/collectors/:id/heartbeat        # 主动探测

# 二进制下载 (dev-zheng)
GET  /api/v1/download/:filename              # 文件下载

# 脚本下载增强 (dev-zheng)
GET  /api/v1/scripts/agentless/setup-terminal.sh
GET  /api/v1/scripts/sysarmor-stack/install-collector.sh
GET  /api/v1/scripts/wazuh-hybrid/install-wazuh.sh
GET  /api/v1/scripts/otelcol/install.sh

# Wazuh 管理 (hfw) - 30+ 个端点
GET  /api/v1/services/wazuh/manager/info
GET  /api/v1/services/wazuh/agents
POST /api/v1/services/wazuh/agents
GET  /api/v1/services/wazuh/events/search
PUT  /api/v1/services/wazuh/config/auth
```

### **数据库模式增强**
```sql
-- 新增字段
ALTER TABLE collectors ADD COLUMN last_active TIMESTAMP;

-- 新增索引
CREATE INDEX idx_collectors_last_active ON collectors(last_active);
CREATE INDEX idx_collectors_status_last_active ON collectors(status, last_active);

-- 新增状态
CollectorStatusOffline = "offline"  -- 长时间无心跳
```

### **配置管理增强**
```bash
# 新增环境变量
DOWNLOAD_DIR=/app/data/dist                   # 二进制文件目录
WAZUH_CONFIG_PATH=./configs/wazuh.yaml        # Wazuh 配置文件
ENV_FILE=.env                                 # 环境变量文件

# 新增配置项
DownloadDir    string  # 下载目录
WazuhConfigPath string # Wazuh 配置路径
```

## 🎯 集成优先级和时间线

### **立即执行 (本周)**
1. ✅ **nova 分支集成** - 双向心跳系统
   - 风险: 低 (主要是功能增强)
   - 收益: 高 (大幅提升监控能力)
   - 工作量: 中等 (15个文件，1000+行代码)

### **短期执行 (下周)**
2. 🔄 **dev-zheng 分支集成** - OpenTelemetry Collector
   - 风险: 低 (独立功能模块)
   - 收益: 中 (扩展数据收集能力)
   - 工作量: 小 (主要是模板和下载功能)

### **中期执行 (下月)**
3. 🔄 **hfw 分支集成** - Wazuh 生态系统
   - 风险: 中 (复杂的外部系统集成)
   - 收益: 高 (完整的 SIEM 集成)
   - 工作量: 大 (4000+行代码，复杂的配置管理)

## 📋 集成检查清单

### **nova 分支集成检查**
- [ ] 数据库迁移执行
- [ ] Collector 模型添加 LastActive 字段
- [ ] 心跳和探测 API 端点实现
- [ ] 安装脚本心跳功能集成
- [ ] dotenv 配置支持
- [ ] Swagger 文档更新

### **dev-zheng 分支集成检查**
- [ ] 下载 API 实现
- [ ] OpenTelemetry 模板集成
- [ ] 二进制文件管理 (Git LFS)
- [ ] Docker 配置更新
- [ ] 脚本路由扩展

### **hfw 分支集成检查**
- [ ] Wazuh 数据模型集成
- [ ] Wazuh 配置管理实现
- [ ] Wazuh API 处理器集成
- [ ] 动态配置更新功能
- [ ] 完整的 Wazuh API 路由

## 🎉 集成完成后的系统能力

### **监控能力增强**
- ✅ **双向心跳机制** - Collector 主动上报 + Manager 主动探测
- ✅ **实时状态监测** - last_active 字段精确跟踪活跃状态
- ✅ **系统健康检查** - rsyslog, auditd, 配置文件状态检查

### **部署能力扩展**
- ✅ **多种部署类型** - agentless, sysarmor-stack, wazuh-hybrid, otel-collector
- ✅ **二进制文件分发** - 安全的文件下载和分发机制
- ✅ **模板系统增强** - 支持多种配置模板和动态参数

### **生态系统集成**
- ✅ **Wazuh 完整集成** - Manager + Indexer + 30+ API 端点
- ✅ **OpenTelemetry 支持** - 标准化的可观测性数据收集
- ✅ **动态配置管理** - 运行时配置更新和连接测试

这个集成方案基于对实际 patch 文件的深入分析，提供了完整、可执行的集成路线图，确保 SysArmor 系统的功能完整性和架构一致性。
