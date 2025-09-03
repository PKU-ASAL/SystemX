# HFW 分支集成计划

## 🎯 集成目标

将 hfw 分支的完整 Wazuh 生态系统集成到当前 Monorepo 架构中，实现 Wazuh Manager + Indexer 的完整 SIEM 集成，扩展 SysArmor 的安全分析能力。

## 📊 HFW 分支核心功能分析

### Wazuh 生态系统集成
```
Wazuh Manager: 代理管理、规则配置、主动响应
Wazuh Indexer: 事件存储、搜索分析、聚合统计
SysArmor Integration: 统一API、配置管理、状态监控
```

### 核心组件
- **30+ API端点**: 完整的Wazuh管理功能
- **动态配置管理**: 运行时配置更新和连接测试
- **事件搜索引擎**: 基于OpenSearch的高级查询
- **代理生命周期管理**: 从注册到删除的完整流程

## 🔧 集成实施计划

### Phase 1: Wazuh 数据模型集成

#### 1.1 Wazuh 核心模型
```go
// 目标文件: sysarmor/apps/manager/models/wazuh.go (1491行)

// Wazuh 代理模型
type WazuhAgent struct {
    ID                string                 `json:"id"`
    Name              string                 `json:"name"`
    IP                string                 `json:"ip"`
    Status            string                 `json:"status"`
    LastKeepAlive     *time.Time             `json:"last_keepalive,omitempty"`
    OS                *WazuhOSInfo           `json:"os,omitempty"`
    Version           string                 `json:"version,omitempty"`
    Manager           string                 `json:"manager,omitempty"`
    DateAdd           *time.Time             `json:"dateAdd,omitempty"`
    Node              string                 `json:"node,omitempty"`
    RegisterIP        string                 `json:"registerIP,omitempty"`
    ConfigSum         string                 `json:"configSum,omitempty"`
    MergedSum         string                 `json:"mergedSum,omitempty"`
    Group             []string               `json:"group,omitempty"`
    Hardware          *WazuhHardwareInfo     `json:"hardware,omitempty"`
    Processes         []WazuhProcessInfo     `json:"processes,omitempty"`
    Packages          []WazuhPackageInfo     `json:"packages,omitempty"`
    Ports             []WazuhPortInfo        `json:"ports,omitempty"`
}

// 系统信息模型
type WazuhOSInfo struct {
    Arch        string `json:"arch,omitempty"`
    Major       string `json:"major,omitempty"`
    Minor       string `json:"minor,omitempty"`
    Name        string `json:"name,omitempty"`
    Platform    string `json:"platform,omitempty"`
    UName       string `json:"uname,omitempty"`
    Version     string `json:"version,omitempty"`
    Codename    string `json:"codename,omitempty"`
    Build       string `json:"build,omitempty"`
}

// 硬件信息模型
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
    Size      int         `json:"size,omitempty"`
    From      int         `json:"from,omitempty"`
    Sort      []map[string]interface{} `json:"sort,omitempty"`
}
```

#### 1.2 错误处理模型
```go
// 目标文件: sysarmor/apps/manager/models/error.go

// Wazuh API 错误模型
type WazuhError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Detail  string `json:"detail,omitempty"`
}

// 通用错误响应
type ErrorResponse struct {
    Success bool        `json:"success"`
    Error   string      `json:"error"`
    Details interface{} `json:"details,omitempty"`
}
```

### Phase 2: Wazuh 配置系统集成

#### 2.1 Wazuh 配置管理
```go
// 目标文件: sysarmor/apps/manager/config/wazuh_config.go (166行)

type WazuhConfig struct {
    Manager  WazuhManagerConfig  `yaml:"manager"`
    Indexer  WazuhIndexerConfig  `yaml:"indexer"`
    Features WazuhFeaturesConfig `yaml:"features"`
}

type WazuhManagerConfig struct {
    URL         string        `yaml:"url"`
    Username    string        `yaml:"username"`
    Password    string        `yaml:"password"`
    Timeout     time.Duration `yaml:"timeout"`
    TokenExpiry time.Duration `yaml:"token_expiry"`
    TLSVerify   bool          `yaml:"tls_verify"`
    MaxRetries  int           `yaml:"max_retries"`
}

type WazuhIndexerConfig struct {
    URL       string                 `yaml:"url"`
    Username  string                 `yaml:"username"`
    Password  string                 `yaml:"password"`
    Timeout   time.Duration          `yaml:"timeout"`
    TLSVerify bool                   `yaml:"tls_verify"`
    Indices   WazuhIndicesConfig     `yaml:"indices"`
}

type WazuhIndicesConfig struct {
    Alerts   string `yaml:"alerts"`   // wazuh-alerts-*
    Archives string `yaml:"archives"` // wazuh-archives-*
}
```

#### 2.2 配置文件
```yaml
# 目标文件: sysarmor/shared/configs/wazuh.yaml

wazuh:
  manager:
    url: "https://wazuh-manager:55000"
    username: "wazuh"
    password: "${WAZUH_MANAGER_PASSWORD}"
    timeout: 30s
    token_expiry: 7200s
    tls_verify: false
    max_retries: 3
  
  indexer:
    url: "https://wazuh-indexer:9200"
    username: "admin"
    password: "${WAZUH_INDEXER_PASSWORD}"
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

### Phase 3: Wazuh 服务层集成

#### 3.1 配置管理器
```go
// 目标文件: sysarmor/apps/manager/services/wazuh/config_manager.go (674行)

type ConfigManager struct {
    staticConfig   *config.WazuhConfig
    dynamicConfig  *models.WazuhDynamicAuthRequest
    managerClient  *ManagerClient
    indexerClient  *IndexerClient
    configFilePath string
    mutex          sync.RWMutex
}

// 动态配置更新
func (cm *ConfigManager) UpdateConfig(ctx context.Context, req *models.WazuhDynamicAuthRequest) error {
    cm.mutex.Lock()
    defer cm.mutex.Unlock()
    
    // 1. 验证新配置
    if err := cm.validateConfig(req); err != nil {
        return fmt.Errorf("config validation failed: %w", err)
    }
    
    // 2. 创建配置备份
    if err := cm.backupConfig(); err != nil {
        return fmt.Errorf("config backup failed: %w", err)
    }
    
    // 3. 测试新配置连接
    if err := cm.testConnections(req); err != nil {
        return fmt.Errorf("connection test failed: %w", err)
    }
    
    // 4. 更新配置文件
    if err := cm.writeConfigFile(req); err != nil {
        return fmt.Errorf("config write failed: %w", err)
    }
    
    // 5. 重新创建客户端
    if err := cm.recreateClients(req); err != nil {
        return fmt.Errorf("client recreation failed: %w", err)
    }
    
    return nil
}
```

#### 3.2 Wazuh Manager 客户端
```go
// 目标文件: sysarmor/apps/manager/services/wazuh/manager_client.go (1459行)

type ManagerClient struct {
    baseURL    string
    username   string
    password   string
    token      string
    tokenExp   time.Time
    httpClient *http.Client
    mutex      sync.RWMutex
}

// 核心方法
func (c *ManagerClient) GetAgents(ctx context.Context, params *AgentQueryParams) (*AgentListResponse, error)
func (c *ManagerClient) AddAgent(ctx context.Context, req *AddAgentRequest) (*AddAgentResponse, error)
func (c *ManagerClient) DeleteAgent(ctx context.Context, agentID string) error
func (c *ManagerClient) RestartAgent(ctx context.Context, agentID string) error
func (c *ManagerClient) GetAgentKey(ctx context.Context, agentID string) (*AgentKeyResponse, error)
func (c *ManagerClient) GetHardwareInfo(ctx context.Context, agentID string) (*WazuhHardwareInfo, error)
func (c *ManagerClient) GetProcesses(ctx context.Context, agentID string) ([]WazuhProcessInfo, error)
func (c *ManagerClient) GetPackages(ctx context.Context, agentID string) ([]WazuhPackageInfo, error)
func (c *ManagerClient) GetPorts(ctx context.Context, agentID string) ([]WazuhPortInfo, error)
```

#### 3.3 Wazuh Indexer 客户端
```go
// 目标文件: sysarmor/apps/manager/services/wazuh/indexer_client.go (591行)

type IndexerClient struct {
    baseURL    string
    username   string
    password   string
    httpClient *http.Client
    indices    WazuhIndicesConfig
}

// 核心方法
func (c *IndexerClient) SearchEvents(ctx context.Context, query *WazuhSearchQuery) (*SearchResponse, error)
func (c *IndexerClient) GetEventByID(ctx context.Context, indexType, eventID string) (*EventResponse, error)
func (c *IndexerClient) GetAggregations(ctx context.Context, query *AggregationQuery) (*AggregationResponse, error)
func (c *IndexerClient) GetIndices(ctx context.Context) ([]IndexInfo, error)
func (c *IndexerClient) GetClusterHealth(ctx context.Context) (*ClusterHealthResponse, error)
```

### Phase 4: Wazuh API 处理器集成

#### 4.1 完整的 Wazuh API 处理器
```go
// 目标文件: sysarmor/apps/manager/api/handlers/wazuh.go (2291行)

type WazuhHandler struct {
    configManager *wazuh.ConfigManager
    managerClient *wazuh.ManagerClient
    indexerClient *wazuh.IndexerClient
}

// Manager API 端点 (20+ 个)
func (h *WazuhHandler) GetManagerInfo(c *gin.Context)
func (h *WazuhHandler) GetManagerStatus(c *gin.Context)
func (h *WazuhHandler) GetAgents(c *gin.Context)
func (h *WazuhHandler) AddAgent(c *gin.Context)
func (h *WazuhHandler) DeleteAgent(c *gin.Context)
func (h *WazuhHandler) RestartAgent(c *gin.Context)
func (h *WazuhHandler) GetAgentKey(c *gin.Context)
func (h *WazuhHandler) GetHardwareInfo(c *gin.Context)
func (h *WazuhHandler) GetProcesses(c *gin.Context)
func (h *WazuhHandler) GetPackages(c *gin.Context)
func (h *WazuhHandler) GetPorts(c *gin.Context)
func (h *WazuhHandler) ActiveResponse(c *gin.Context)

// Indexer API 端点 (10+ 个)
func (h *WazuhHandler) SearchEvents(c *gin.Context)
func (h *WazuhHandler) GetEventByID(c *gin.Context)
func (h *WazuhHandler) GetAggregations(c *gin.Context)
func (h *WazuhHandler) GetIndices(c *gin.Context)
func (h *WazuhHandler) GetClusterHealth(c *gin.Context)

// 配置管理 API
func (h *WazuhHandler) UpdateWazuhAuth(c *gin.Context)
func (h *WazuhHandler) GetWazuhAuth(c *gin.Context)
func (h *WazuhHandler) TestWazuhConnection(c *gin.Context)
```

### Phase 5: 路由配置扩展

#### 5.1 Wazuh API 路由组
```go
// 目标文件: sysarmor/apps/manager/main.go

// Wazuh 管理路由 (hfw 分支集成)
wazuhHandler := handlers.NewWazuhHandler(cfg.GetWazuhConfig())
wazuh := services.Group("/wazuh")
{
    // Manager 管理
    manager := wazuh.Group("/manager")
    {
        manager.GET("/info", wazuhHandler.GetManagerInfo)
        manager.GET("/status", wazuhHandler.GetManagerStatus)
        manager.GET("/configuration", wazuhHandler.GetManagerConfiguration)
        manager.GET("/logs", wazuhHandler.GetManagerLogs)
    }
    
    // 代理管理
    agents := wazuh.Group("/agents")
    {
        agents.GET("", wazuhHandler.GetAgents)
        agents.POST("", wazuhHandler.AddAgent)
        agents.GET("/:id", wazuhHandler.GetAgentDetails)
        agents.DELETE("/:id", wazuhHandler.DeleteAgent)
        agents.PUT("/:id/restart", wazuhHandler.RestartAgent)
        agents.GET("/:id/key", wazuhHandler.GetAgentKey)
        agents.GET("/:id/hardware", wazuhHandler.GetHardwareInfo)
        agents.GET("/:id/processes", wazuhHandler.GetProcesses)
        agents.GET("/:id/packages", wazuhHandler.GetPackages)
        agents.GET("/:id/ports", wazuhHandler.GetPorts)
        agents.PUT("/:id/active-response", wazuhHandler.ActiveResponse)
    }
    
    // 事件搜索和分析
    events := wazuh.Group("/events")
    {
        events.POST("/search", wazuhHandler.SearchEvents)
        events.GET("/:index_type/:id", wazuhHandler.GetEventByID)
        events.POST("/aggregations", wazuhHandler.GetAggregations)
        events.GET("/recent", wazuhHandler.GetRecentEvents)
        events.GET("/alerts/high-priority", wazuhHandler.GetHighPriorityAlerts)
    }
    
    // 索引管理
    indices := wazuh.Group("/indices")
    {
        indices.GET("", wazuhHandler.GetIndices)
        indices.GET("/stats", wazuhHandler.GetIndicesStats)
        indices.GET("/health", wazuhHandler.GetClusterHealth)
    }
    
    // 配置管理
    config := wazuh.Group("/config")
    {
        config.GET("/auth", wazuhHandler.GetWazuhAuth)
        config.PUT("/auth", wazuhHandler.UpdateWazuhAuth)
        config.POST("/test", wazuhHandler.TestWazuhConnection)
        config.GET("/backup", wazuhHandler.GetConfigBackup)
        config.POST("/restore", wazuhHandler.RestoreConfig)
    }
}
```

### Phase 6: 环境变量和配置扩展

#### 6.1 新增环境变量
```bash
# 目标文件: sysarmor/.env

# Wazuh Manager 配置
WAZUH_MANAGER_URL=https://wazuh-manager:55000
WAZUH_MANAGER_USERNAME=wazuh
WAZUH_MANAGER_PASSWORD=your-secure-password
WAZUH_MANAGER_TIMEOUT=30s
WAZUH_MANAGER_TOKEN_EXPIRY=7200s
WAZUH_MANAGER_TLS_VERIFY=false
WAZUH_MANAGER_MAX_RETRIES=3

# Wazuh Indexer 配置
WAZUH_INDEXER_URL=https://wazuh-indexer:9200
WAZUH_INDEXER_USERNAME=admin
WAZUH_INDEXER_PASSWORD=your-secure-password
WAZUH_INDEXER_TIMEOUT=30s
WAZUH_INDEXER_TLS_VERIFY=false
WAZUH_INDEXER_MAX_RETRIES=3

# Wazuh 索引配置
WAZUH_ALERTS_INDEX=wazuh-alerts-*
WAZUH_ARCHIVES_INDEX=wazuh-archives-*

# Wazuh 功能开关
WAZUH_MANAGER_ENABLED=true
WAZUH_INDEXER_ENABLED=true
WAZUH_AUTO_SYNC=true
WAZUH_SYNC_INTERVAL=300s

# Wazuh 配置文件路径
WAZUH_CONFIG_PATH=./shared/configs/wazuh.yaml
```

#### 6.2 配置加载扩展
```go
// 目标文件: sysarmor/apps/manager/config/config.go

type Config struct {
    // 现有字段...
    WazuhConfigPath string `env:"WAZUH_CONFIG_PATH" envDefault:"./shared/configs/wazuh.yaml"`
}

// 新增方法
func (c *Config) GetWazuhConfig() (*WazuhConfig, error) {
    if c.WazuhConfigPath == "" {
        return nil, fmt.Errorf("wazuh config path not set")
    }
    
    data, err := os.ReadFile(c.WazuhConfigPath)
    if err != nil {
        return nil, fmt.Errorf("failed to read wazuh config: %w", err)
    }
    
    var wazuhConfig WazuhConfig
    if err := yaml.Unmarshal(data, &wazuhConfig); err != nil {
        return nil, fmt.Errorf("failed to parse wazuh config: %w", err)
    }
    
    return &wazuhConfig, nil
}
```

## 🧪 集成测试计划

### 测试用例设计
```bash
# 1. Wazuh Manager 连接测试
curl -X POST http://localhost:8080/api/v1/services/wazuh/config/test \
  -d '{"manager_url":"https://wazuh-manager:55000","username":"wazuh","password":"test"}'

# 2. 代理管理测试
curl "http://localhost:8080/api/v1/services/wazuh/agents"
curl -X POST http://localhost:8080/api/v1/services/wazuh/agents \
  -d '{"name":"test-agent","ip":"192.168.1.100"}'

# 3. 事件搜索测试
curl -X POST http://localhost:8080/api/v1/services/wazuh/events/search \
  -d '{"index_type":"alerts","query":"*","size":10}'

# 4. 配置管理测试
curl -X PUT http://localhost:8080/api/v1/services/wazuh/config/auth \
  -d '{"manager":{"url":"https://new-url:55000","username":"admin","password":"newpass"}}'
```

### 集成测试脚本
```bash
# 目标文件: sysarmor/tests/migrations/test-hfw.sh

#!/bin/bash
# HFW 分支 Wazuh 集成功能测试

# 1. 测试 Wazuh Manager 连接
# 2. 测试代理管理功能
# 3. 测试事件搜索功能
# 4. 测试配置管理功能
# 5. 测试错误处理和重试机制
```

## 📋 实施步骤

### Step 1: 数据模型集成 (优先级: 高)
- [ ] 创建 `sysarmor/apps/manager/models/wazuh.go`
- [ ] 创建 `sysarmor/apps/manager/models/error.go`
- [ ] 添加 Wazuh 相关的数据结构和验证

### Step 2: 配置系统集成 (优先级: 高)
- [ ] 创建 `sysarmor/apps/manager/config/wazuh_config.go`
- [ ] 创建 `sysarmor/shared/configs/wazuh.yaml`
- [ ] 扩展主配置文件支持 Wazuh 配置路径
- [ ] 添加环境变量支持

### Step 3: 服务层实现 (优先级: 中)
- [ ] 实现 `ConfigManager` 动态配置管理
- [ ] 实现 `ManagerClient` Wazuh Manager 客户端
- [ ] 实现 `IndexerClient` Wazuh Indexer 客户端
- [ ] 添加连接池和重试机制

### Step 4: API 处理器实现 (优先级: 中)
- [ ] 实现 `WazuhHandler` 完整的API处理器
- [ ] 添加 30+ 个 Wazuh API 端点
- [ ] 实现错误处理和响应格式化
- [ ] 添加请求验证和安全检查

### Step 5: 路由配置更新 (优先级: 中)
- [ ] 添加完整的 Wazuh API 路由组
- [ ] 集成到主路由配置
- [ ] 添加中间件和认证

### Step 6: 测试和验证 (优先级: 高)
- [ ] 创建 HFW 集成测试脚本
- [ ] 验证所有 Wazuh API 功能
- [ ] 测试配置管理和动态更新
- [ ] 验证错误处理和重试机制

## 🔄 迁移策略

### 向后兼容性
```go
// Wazuh 功能作为可选扩展
if cfg.WazuhEnabled {
    // 只有启用 Wazuh 时才注册相关路由
    wazuhHandler := handlers.NewWazuhHandler(cfg.GetWazuhConfig())
    // 注册 Wazuh 路由...
}
```

### 渐进式部署
```bash
# 阶段1: 基础配置和模型
- 添加 Wazuh 配置文件和数据模型
- 不启用 Wazuh 功能，保持现有系统不变

# 阶段2: 服务层集成
- 实现 Wazuh 客户端和配置管理
- 添加连接测试功能

# 阶段3: API 端点实现
- 逐步添加 Wazuh API 端点
- 测试每个功能模块

# 阶段4: 完整功能启用
- 启用所有 Wazuh 功能
- 完整的集成测试验证
```

## 🎯 预期收益

### SIEM 能力扩展
- **完整的代理管理**: 从注册到删除的生命周期管理
- **高级事件搜索**: 基于 OpenSearch 的复杂查询能力
- **实时告警分析**: Wazuh 规则引擎集成
- **系统信息收集**: 硬件、进程、软件包、端口信息

### 运维效率提升
- **统一管理界面**: 通过 SysArmor API 管理 Wazuh 组件
- **动态配置管理**: 运行时更新 Wazuh 连接配置
- **自动化部署**: 支持 wazuh-hybrid 部署类型
- **监控集成**: Wazuh 状态集成到 SysArmor 监控体系

## 🔮 风险评估

### 技术风险 (中)
- **外部依赖**: 依赖 Wazuh Manager 和 Indexer 服务
- **网络连接**: 需要稳定的网络连接到 Wazuh 组件
- **配置复杂性**: Wazuh 配置相对复杂，需要仔细管理
- **版本兼容性**: 需要确保与 Wazuh 版本的兼容性

### 兼容性风险 (低)
- **可选功能**: Wazuh 集成作为可选扩展，不影响现有功能
- **独立模块**: Wazuh 相关代码独立，不影响核心功能
- **配置驱动**: 通过配置开关控制功能启用

## 📅 实施时间线

### 第1周: 基础架构
- Day 1-2: 数据模型和配置系统
- Day 3-4: 基础服务层实现
- Day 5: 连接测试和验证

### 第2周: API 实现
- Day 1-3: Manager API 端点实现
- Day 4-5: Indexer API 端点实现

### 第3周: 集成和测试
- Day 1-2: 路由配置和集成
- Day 3-4: 完整测试和验证
- Day 5: 文档和优化

### 第4周: 部署和优化
- Day 1-2: 生产环境部署
- Day 3-5: 监控和性能优化

## 🎯 集成后的系统架构

### 扩展的部署类型
```go
const (
    DeploymentTypeAgentless = "agentless"      // 现有: rsyslog/auditd
    DeploymentTypeSysArmor  = "sysarmor-stack" // 现有: 完整栈
    DeploymentTypeWazuh     = "wazuh-hybrid"   // 扩展: Wazuh集成
    DeploymentTypeCollector = "collector"      // 现有: OpenTelemetry
)
```

### API 端点总览
```
# 现有 SysArmor API
/api/v1/collectors/*          # Collector 管理
/api/v1/resources/*           # 资源下载
/api/v1/health/*              # 健康检查
/api/v1/events/*              # 事件查询

# 新增 Wazuh API
/api/v1/services/wazuh/manager/*    # Wazuh Manager 管理
/api/v1/services/wazuh/agents/*     # Wazuh 代理管理
/api/v1/services/wazuh/events/*     # Wazuh 事件搜索
/api/v1/services/wazuh/indices/*    # Wazuh 索引管理
/api/v1/services/wazuh/config/*     # Wazuh 配置管理
```

### 配置管理架构
```
SysArmor Config (主配置)
├── Manager Config (现有)
├── Middleware Config (现有)
├── Processor Config (现有)
└── Wazuh Config (新增)
    ├── Manager Config
    ├── Indexer Config
    └── Features Config
```

---

**HFW 分支集成总结**: 通过完整的 Wazuh 生态系统集成，SysArmor 将获得强大的 SIEM 能力，包括代理管理、事件搜索、配置管理等30+个API端点，大幅扩展安全分析和监控能力。
