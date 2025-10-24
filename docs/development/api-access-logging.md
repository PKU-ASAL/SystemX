# SysArmor Manager API 访问日志记录功能设计

## 概述

本文档设计了一套完整的Manager API访问日志记录系统，用于记录、分析和监控所有对Manager API的访问请求，提供安全审计、性能分析和运营监控能力。

## 功能需求

### 核心需求
- **全量记录** - 记录所有API请求和响应
- **结构化存储** - 便于查询和分析的数据格式
- **性能优化** - 不影响API响应性能
- **安全审计** - 支持安全事件追溯
- **运营监控** - 提供API使用统计和性能分析

### 扩展需求
- **实时监控** - 支持实时API访问监控
- **异常检测** - 识别异常访问模式
- **报表生成** - 定期生成访问统计报表
- **告警机制** - 异常访问自动告警

## 技术方案

### 1. Gin Middleware实现

#### 1.1 中间件架构
```go
// middleware/logging.go
package middleware

import (
    "bytes"
    "encoding/json"
    "io"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

type APIAccessLog struct {
    // 请求标识
    RequestID     string    `json:"request_id"`
    Timestamp     time.Time `json:"timestamp"`
    
    // 请求信息
    Method        string    `json:"method"`
    Path          string    `json:"path"`
    Query         string    `json:"query,omitempty"`
    UserAgent     string    `json:"user_agent,omitempty"`
    
    // 客户端信息
    ClientIP      string    `json:"client_ip"`
    XForwardedFor string    `json:"x_forwarded_for,omitempty"`
    XRealIP       string    `json:"x_real_ip,omitempty"`
    
    // 请求体 (可选，敏感接口可能需要脱敏)
    RequestBody   string    `json:"request_body,omitempty"`
    RequestSize   int64     `json:"request_size"`
    
    // 响应信息
    StatusCode    int       `json:"status_code"`
    ResponseSize  int64     `json:"response_size"`
    ResponseTime  int64     `json:"response_time_ms"`
    
    // 错误信息
    Error         string    `json:"error,omitempty"`
    
    // 业务信息
    CollectorID   string    `json:"collector_id,omitempty"`
    UserID        string    `json:"user_id,omitempty"`
    SessionID     string    `json:"session_id,omitempty"`
    
    // 分类标签
    Category      string    `json:"category"` // health, collector, opensearch, kafka, flink
    Sensitive     bool      `json:"sensitive"` // 是否包含敏感信息
}
```

#### 1.2 中间件实现
```go
func APILoggingMiddleware(logger *APILogger) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        requestID := uuid.New().String()
        
        // 设置请求ID到上下文
        c.Set("request_id", requestID)
        c.Header("X-Request-ID", requestID)
        
        // 读取请求体 (如果需要记录)
        var requestBody []byte
        if shouldLogRequestBody(c.Request.URL.Path) {
            if c.Request.Body != nil {
                requestBody, _ = io.ReadAll(c.Request.Body)
                c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
            }
        }
        
        // 创建响应写入器包装器
        writer := &responseWriter{
            ResponseWriter: c.Writer,
            body:          &bytes.Buffer{},
        }
        c.Writer = writer
        
        // 处理请求
        c.Next()
        
        // 计算响应时间
        duration := time.Since(start)
        
        // 构建日志记录
        logEntry := &APIAccessLog{
            RequestID:     requestID,
            Timestamp:     start,
            Method:        c.Request.Method,
            Path:          c.Request.URL.Path,
            Query:         c.Request.URL.RawQuery,
            UserAgent:     c.Request.UserAgent(),
            ClientIP:      c.ClientIP(),
            XForwardedFor: c.Request.Header.Get("X-Forwarded-For"),
            XRealIP:       c.Request.Header.Get("X-Real-IP"),
            RequestBody:   string(requestBody),
            RequestSize:   c.Request.ContentLength,
            StatusCode:    c.Writer.Status(),
            ResponseSize:  int64(writer.body.Len()),
            ResponseTime:  duration.Milliseconds(),
            Category:      categorizeRequest(c.Request.URL.Path),
            Sensitive:     isSensitiveEndpoint(c.Request.URL.Path),
        }
        
        // 提取业务信息
        if collectorID := c.Param("id"); collectorID != "" {
            logEntry.CollectorID = collectorID
        }
        
        // 记录错误信息
        if len(c.Errors) > 0 {
            logEntry.Error = c.Errors.String()
        }
        
        // 异步记录日志
        logger.LogAsync(logEntry)
    }
}
```

### 2. 日志存储方案

#### 2.1 数据库表设计
```sql
-- API访问日志表
CREATE TABLE api_access_logs (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(36) UNIQUE NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    
    -- 请求信息
    method VARCHAR(10) NOT NULL,
    path VARCHAR(500) NOT NULL,
    query TEXT,
    user_agent TEXT,
    
    -- 客户端信息
    client_ip INET NOT NULL,
    x_forwarded_for VARCHAR(255),
    x_real_ip VARCHAR(255),
    
    -- 请求响应信息
    request_body TEXT,
    request_size BIGINT DEFAULT 0,
    status_code INTEGER NOT NULL,
    response_size BIGINT DEFAULT 0,
    response_time_ms BIGINT NOT NULL,
    
    -- 错误信息
    error_message TEXT,
    
    -- 业务信息
    collector_id VARCHAR(255),
    user_id VARCHAR(255),
    session_id VARCHAR(255),
    
    -- 分类信息
    category VARCHAR(50) NOT NULL,
    sensitive BOOLEAN DEFAULT FALSE,
    
    -- 索引字段
    created_at TIMESTAMP DEFAULT NOW()
);

-- 索引优化
CREATE INDEX idx_api_logs_timestamp ON api_access_logs(timestamp);
CREATE INDEX idx_api_logs_path ON api_access_logs(path);
CREATE INDEX idx_api_logs_status_code ON api_access_logs(status_code);
CREATE INDEX idx_api_logs_client_ip ON api_access_logs(client_ip);
CREATE INDEX idx_api_logs_collector_id ON api_access_logs(collector_id);
CREATE INDEX idx_api_logs_category ON api_access_logs(category);
CREATE INDEX idx_api_logs_response_time ON api_access_logs(response_time_ms);

-- 复合索引
CREATE INDEX idx_api_logs_category_timestamp ON api_access_logs(category, timestamp);
CREATE INDEX idx_api_logs_status_timestamp ON api_access_logs(status_code, timestamp);
```

#### 2.2 多存储策略
```go
type LogStorage interface {
    Store(log *APIAccessLog) error
    Query(filter *LogFilter) ([]*APIAccessLog, error)
}

// 数据库存储
type DatabaseStorage struct {
    db *sql.DB
}

// 文件存储 (备份/归档)
type FileStorage struct {
    logDir string
}

// OpenSearch存储 (高级分析)
type OpenSearchStorage struct {
    client *opensearch.Client
}

// 组合存储策略
type MultiStorage struct {
    primary   LogStorage  // 主存储 (数据库)
    secondary []LogStorage // 辅助存储 (文件、OpenSearch)
}
```

### 3. 日志分类和过滤

#### 3.1 请求分类
```go
func categorizeRequest(path string) string {
    switch {
    case strings.HasPrefix(path, "/health"):
        return "health"
    case strings.HasPrefix(path, "/api/v1/collectors"):
        return "collector"
    case strings.HasPrefix(path, "/api/v1/services/opensearch"):
        return "opensearch"
    case strings.HasPrefix(path, "/api/v1/services/kafka"):
        return "kafka"
    case strings.HasPrefix(path, "/api/v1/services/flink"):
        return "flink"
    case strings.HasPrefix(path, "/api/v1/events"):
        return "events"
    case strings.HasPrefix(path, "/api/v1/dashboard"):
        return "dashboard"
    case strings.HasPrefix(path, "/api/v1/wazuh"):
        return "wazuh"
    default:
        return "other"
    }
}
```

#### 3.2 敏感信息处理
```go
func isSensitiveEndpoint(path string) bool {
    sensitivePatterns := []string{
        "/api/v1/collectors/register",
        "/api/v1/collectors/.*/heartbeat",
        "/api/v1/auth/",
        "/api/v1/config/",
    }
    
    for _, pattern := range sensitivePatterns {
        if matched, _ := regexp.MatchString(pattern, path); matched {
            return true
        }
    }
    return false
}

func sanitizeRequestBody(body string, path string) string {
    if !isSensitiveEndpoint(path) {
        return body
    }
    
    // 脱敏处理
    var data map[string]interface{}
    if err := json.Unmarshal([]byte(body), &data); err != nil {
        return "[REDACTED]"
    }
    
    // 移除敏感字段
    sensitiveFields := []string{"password", "token", "secret", "key"}
    for _, field := range sensitiveFields {
        if _, exists := data[field]; exists {
            data[field] = "[REDACTED]"
        }
    }
    
    sanitized, _ := json.Marshal(data)
    return string(sanitized)
}
```

### 4. 异步日志处理

#### 4.1 异步写入机制
```go
type APILogger struct {
    storage   LogStorage
    logChan   chan *APIAccessLog
    batchSize int
    flushInterval time.Duration
    buffer    []*APIAccessLog
    mutex     sync.Mutex
}

func NewAPILogger(storage LogStorage) *APILogger {
    logger := &APILogger{
        storage:       storage,
        logChan:       make(chan *APIAccessLog, 1000),
        batchSize:     100,
        flushInterval: 5 * time.Second,
        buffer:        make([]*APIAccessLog, 0, 100),
    }
    
    go logger.processLogs()
    return logger
}

func (l *APILogger) LogAsync(log *APIAccessLog) {
    select {
    case l.logChan <- log:
    default:
        // 如果通道满了，记录到错误日志
        fmt.Printf("Warning: API log channel full, dropping log entry\n")
    }
}

func (l *APILogger) processLogs() {
    ticker := time.NewTicker(l.flushInterval)
    defer ticker.Stop()
    
    for {
        select {
        case log := <-l.logChan:
            l.addToBuffer(log)
            if len(l.buffer) >= l.batchSize {
                l.flushBuffer()
            }
        case <-ticker.C:
            l.flushBuffer()
        }
    }
}
```

### 5. 查询和分析接口

#### 5.1 日志查询API
```go
// GET /api/v1/logs/access
func (h *LogHandler) QueryAccessLogs(c *gin.Context) {
    filter := &LogFilter{
        StartTime:   parseTime(c.Query("start_time")),
        EndTime:     parseTime(c.Query("end_time")),
        Method:      c.Query("method"),
        Path:        c.Query("path"),
        StatusCode:  parseInt(c.Query("status_code")),
        ClientIP:    c.Query("client_ip"),
        CollectorID: c.Query("collector_id"),
        Category:    c.Query("category"),
        Page:        parseInt(c.DefaultQuery("page", "1")),
        Limit:       parseInt(c.DefaultQuery("limit", "50")),
    }
    
    logs, total, err := h.storage.Query(filter)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{
        "success": true,
        "data": gin.H{
            "logs":  logs,
            "total": total,
            "page":  filter.Page,
            "limit": filter.Limit,
        },
    })
}
```

#### 5.2 统计分析API
```go
// GET /api/v1/logs/stats
func (h *LogHandler) GetAccessStats(c *gin.Context) {
    timeRange := c.DefaultQuery("time_range", "24h")
    
    stats, err := h.analyzer.GetStats(timeRange)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    
    c.JSON(200, gin.H{
        "success": true,
        "data": stats,
    })
}

type AccessStats struct {
    TimeRange string `json:"time_range"`
    
    // 请求统计
    TotalRequests    int64   `json:"total_requests"`
    RequestsPerHour  float64 `json:"requests_per_hour"`
    
    // 状态码分布
    StatusCodes map[string]int64 `json:"status_codes"`
    
    // 方法分布
    Methods map[string]int64 `json:"methods"`
    
    // 分类分布
    Categories map[string]int64 `json:"categories"`
    
    // 性能指标
    AvgResponseTime int64 `json:"avg_response_time_ms"`
    P95ResponseTime int64 `json:"p95_response_time_ms"`
    P99ResponseTime int64 `json:"p99_response_time_ms"`
    
    // 错误统计
    ErrorRate       float64 `json:"error_rate"`
    TotalErrors     int64   `json:"total_errors"`
    
    // 热门端点
    TopEndpoints []EndpointStat `json:"top_endpoints"`
    
    // 活跃客户端
    TopClients []ClientStat `json:"top_clients"`
}
```

### 6. 配置管理

#### 6.1 配置结构
```go
type LoggingConfig struct {
    Enabled       bool          `yaml:"enabled" json:"enabled"`
    Level         string        `yaml:"level" json:"level"` // all, errors_only, none
    
    // 存储配置
    Storage struct {
        Database    bool   `yaml:"database" json:"database"`
        File        bool   `yaml:"file" json:"file"`
        OpenSearch  bool   `yaml:"opensearch" json:"opensearch"`
        FileDir     string `yaml:"file_dir" json:"file_dir"`
    } `yaml:"storage" json:"storage"`
    
    // 性能配置
    Performance struct {
        BatchSize     int           `yaml:"batch_size" json:"batch_size"`
        FlushInterval time.Duration `yaml:"flush_interval" json:"flush_interval"`
        ChannelSize   int           `yaml:"channel_size" json:"channel_size"`
    } `yaml:"performance" json:"performance"`
    
    // 过滤配置
    Filters struct {
        ExcludePaths    []string `yaml:"exclude_paths" json:"exclude_paths"`
        IncludeBody     []string `yaml:"include_body" json:"include_body"`
        SensitivePaths  []string `yaml:"sensitive_paths" json:"sensitive_paths"`
    } `yaml:"filters" json:"filters"`
    
    // 保留策略
    Retention struct {
        Days        int  `yaml:"days" json:"days"`
        AutoCleanup bool `yaml:"auto_cleanup" json:"auto_cleanup"`
    } `yaml:"retention" json:"retention"`
}
```

#### 6.2 默认配置
```yaml
# config/logging.yaml
logging:
  enabled: true
  level: "all"  # all, errors_only, none
  
  storage:
    database: true
    file: true
    opensearch: false
    file_dir: "./data/logs/api"
  
  performance:
    batch_size: 100
    flush_interval: "5s"
    channel_size: 1000
  
  filters:
    exclude_paths:
      - "/health"
      - "/metrics"
      - "/favicon.ico"
    include_body:
      - "/api/v1/collectors/register"
      - "/api/v1/collectors/*/heartbeat"
    sensitive_paths:
      - "/api/v1/auth/*"
      - "/api/v1/config/*"
  
  retention:
    days: 30
    auto_cleanup: true
```

### 7. 监控和告警

#### 7.1 实时监控
```go
type LogMonitor struct {
    alertThresholds map[string]float64
    windowSize      time.Duration
    alertChannel    chan Alert
}

type Alert struct {
    Type        string    `json:"type"`
    Message     string    `json:"message"`
    Severity    string    `json:"severity"`
    Timestamp   time.Time `json:"timestamp"`
    Metadata    map[string]interface{} `json:"metadata"`
}

// 监控规则
var defaultAlertRules = map[string]float64{
    "error_rate_5min":      0.05,  // 5分钟错误率超过5%
    "response_time_p95":    5000,  // P95响应时间超过5秒
    "requests_per_minute":  1000,  // 每分钟请求数超过1000
    "failed_auth_rate":     0.1,   // 认证失败率超过10%
}
```

#### 7.2 异常检测
```go
func (m *LogMonitor) detectAnomalies(logs []*APIAccessLog) []Alert {
    var alerts []Alert
    
    // 检测异常IP访问
    if suspiciousIPs := m.detectSuspiciousIPs(logs); len(suspiciousIPs) > 0 {
        alerts = append(alerts, Alert{
            Type:     "suspicious_ip",
            Message:  fmt.Sprintf("Detected %d suspicious IP addresses", len(suspiciousIPs)),
            Severity: "medium",
            Metadata: map[string]interface{}{"ips": suspiciousIPs},
        })
    }
    
    // 检测暴力破解
    if bruteForceAttempts := m.detectBruteForce(logs); len(bruteForceAttempts) > 0 {
        alerts = append(alerts, Alert{
            Type:     "brute_force",
            Message:  "Detected potential brute force attacks",
            Severity: "high",
            Metadata: map[string]interface{}{"attempts": bruteForceAttempts},
        })
    }
    
    return alerts
}
```

### 8. 日志分析和报表

#### 8.1 统计分析器
```go
type LogAnalyzer struct {
    storage LogStorage
}

func (a *LogAnalyzer) GenerateReport(timeRange string) (*AccessReport, error) {
    filter := &LogFilter{
        StartTime: parseTimeRange(timeRange),
        EndTime:   time.Now(),
    }
    
    logs, _, err := a.storage.Query(filter)
    if err != nil {
        return nil, err
    }
    
    return &AccessReport{
        TimeRange:       timeRange,
        TotalRequests:   len(logs),
        UniqueIPs:       a.countUniqueIPs(logs),
        TopEndpoints:    a.getTopEndpoints(logs, 10),
        ErrorAnalysis:   a.analyzeErrors(logs),
        PerformanceMetrics: a.calculatePerformance(logs),
        SecurityEvents:  a.detectSecurityEvents(logs),
    }, nil
}
```

#### 8.2 定期报表生成
```go
func (s *ReportScheduler) Start() {
    // 每日报表
    dailyTicker := time.NewTicker(24 * time.Hour)
    go func() {
        for range dailyTicker.C {
            s.generateDailyReport()
        }
    }()
    
    // 每周报表
    weeklyTicker := time.NewTicker(7 * 24 * time.Hour)
    go func() {
        for range weeklyTicker.C {
            s.generateWeeklyReport()
        }
    }()
}
```

## 实现计划

### 🔥 第一阶段 (核心功能)
**预计时间: 2-3天**

- [ ] 实现基础Gin中间件
- [ ] 设计数据库表结构
- [ ] 实现异步日志写入
- [ ] 基础查询API
- [ ] 配置管理

**技术要求:**
- Gin中间件开发
- PostgreSQL表设计
- Go并发编程
- 配置文件解析

### ⚡ 第二阶段 (增强功能)
**预计时间: 3-4天**

- [ ] 实现多存储策略
- [ ] 日志统计分析API
- [ ] 实时监控功能
- [ ] 基础告警机制
- [ ] 性能优化

**技术要求:**
- 多存储适配器模式
- 统计分析算法
- 实时数据处理
- 性能调优

### 🎯 第三阶段 (高级功能)
**预计时间: 1周**

- [ ] 异常检测算法
- [ ] 自动报表生成
- [ ] OpenSearch集成
- [ ] 前端日志查看界面
- [ ] 告警通知机制

**技术要求:**
- 机器学习算法
- 报表生成引擎
- 前端数据可视化
- 通知系统集成

## 使用示例

### 启用日志记录
```go
// main.go
func main() {
    r := gin.New()
    
    // 配置日志记录
    logConfig := loadLoggingConfig()
    logger := NewAPILogger(logConfig)
    
    // 注册中间件
    r.Use(APILoggingMiddleware(logger))
    r.Use(gin.Recovery())
    
    // 注册路由
    setupRoutes(r)
    
    r.Run(":8080")
}
```

### 查询访问日志
```bash
# 查询最近24小时的错误请求
curl "http://localhost:8080/api/v1/logs/access?start_time=2025-09-23T00:00:00Z&status_code=500"

# 查询特定Collector的访问记录
curl "http://localhost:8080/api/v1/logs/access?collector_id=collector-001&limit=100"

# 获取访问统计
curl "http://localhost:8080/api/v1/logs/stats?time_range=7d"
```

### 配置示例
```yaml
# 生产环境配置
logging:
  enabled: true
  level: "all"
  storage:
    database: true
    file: true
    opensearch: true
  retention:
    days: 90
    auto_cleanup: true

# 开发环境配置
logging:
  enabled: true
  level: "errors_only"
  storage:
    database: true
    file: false
    opensearch: false
  retention:
    days: 7
```

## 安全考虑

### 数据保护
- **敏感信息脱敏** - 自动识别和脱敏敏感字段
- **访问控制** - 日志查询需要适当权限
- **数据加密** - 敏感日志数据加密存储
- **审计追踪** - 日志访问本身也需要记录

### 合规性
- **数据保留** - 符合法规要求的数据保留策略
- **隐私保护** - 个人信息的匿名化处理
- **审计标准** - 符合安全审计标准
- **数据导出** - 支持合规性检查的数据导出

## 性能影响评估

### 预期性能影响
- **延迟增加** - 预计增加1-3ms请求延迟
- **内存使用** - 增加约10-20MB内存使用
- **存储需求** - 每天约100MB-1GB日志数据
- **CPU开销** - 增加约2-5%CPU使用率

### 优化策略
- **异步处理** - 所有日志写入异步执行
- **批量写入** - 批量写入数据库减少IO
- **索引优化** - 合理的数据库索引设计
- **数据压缩** - 历史数据压缩存储

这个设计提供了完整的API访问日志记录解决方案，既满足了安全审计需求，又保持了良好的性能和可扩展性。
