package health

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// HealthChecker 健康检查器
type HealthChecker struct {
	client  *http.Client
	workers []WorkerConfig
}

// WorkerConfig Worker 配置
type WorkerConfig struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	HealthURL string `json:"health_url"`
}

// WorkerHealthResult Worker 健康检查结果
type WorkerHealthResult struct {
	Name         string        `json:"name"`
	URL          string        `json:"url"`
	Healthy      bool          `json:"healthy"`
	ResponseTime time.Duration `json:"response_time_ms"`
	Error        string        `json:"error,omitempty"`
	CheckedAt    time.Time     `json:"checked_at"`
	
	// 扩展的健康信息
	Components   *WorkerComponents `json:"components,omitempty"`
	Metrics      *WorkerMetrics    `json:"metrics,omitempty"`
	Version      string            `json:"version,omitempty"`
}

// WorkerComponents Worker 组件状态
type WorkerComponents struct {
	Total      int                    `json:"total"`
	Active     int                    `json:"active"`
	Failed     int                    `json:"failed"`
	Components []ComponentStatus      `json:"components"`
}

// ComponentStatus 组件状态
type ComponentStatus struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	EventsTotal int64     `json:"events_total"`
	BytesTotal  int64     `json:"bytes_total"`
	ErrorsTotal int64     `json:"errors_total"`
	LastActive  time.Time `json:"last_active"`
}

// WorkerMetrics Worker 关键指标
type WorkerMetrics struct {
	// 数据指标 (只统计 source 组件)
	EventsReceived  int64   `json:"events_received"`
	EventsProcessed int64   `json:"events_processed"`
	BytesReceived   int64   `json:"bytes_received"`
	BytesProcessed  int64   `json:"bytes_processed"`
	
	// 错误和性能指标
	ErrorsTotal           int64   `json:"errors_total"`
	BufferUtilization     float64 `json:"buffer_utilization"`
	ProcessingLatency     float64 `json:"processing_latency_ms"`
	
	// 数据源统计 (便于理解数据来源)
	DataSources map[string]ComponentMetrics `json:"data_sources,omitempty"`
}

// ComponentMetrics 组件指标
type ComponentMetrics struct {
	EventsReceived int64 `json:"events_received"`
	BytesReceived  int64 `json:"bytes_received"`
	ErrorsTotal    int64 `json:"errors_total"`
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker() *HealthChecker {
	checker := &HealthChecker{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
	
	// 从环境变量加载 worker 配置
	checker.loadWorkersFromEnv()
	
	return checker
}

// loadWorkersFromEnv 从环境变量加载 worker 配置
func (h *HealthChecker) loadWorkersFromEnv() {
	// 从环境变量读取 worker 配置
	// 格式: WORKER_URLS=worker1:http://192.168.1.200:514:http://192.168.1.200:8686/health,worker2:http://192.168.1.201:514:http://192.168.1.201:8686/health
	workersEnv := os.Getenv("WORKER_URLS")
	if workersEnv == "" {
		// 默认配置
		h.workers = []WorkerConfig{
			{
				Name:      "default-worker",
				URL:       "http://localhost:514",
				HealthURL: "http://localhost:8686/health",
			},
		}
		return
	}

	// 解析环境变量
	workerEntries := strings.Split(workersEnv, ",")
	for _, entry := range workerEntries {
		parts := strings.Split(strings.TrimSpace(entry), ":")
		if len(parts) >= 4 { // name:http://host:port:http://host:port/health
			name := parts[0]
			
			// 重新组合 URL 部分
			remaining := strings.Join(parts[1:], ":")
			
			// 查找最后一个 http:// 来分离数据 URL 和健康检查 URL
			lastHttpIndex := strings.LastIndex(remaining, "http://")
			if lastHttpIndex > 0 {
				dataURL := remaining[:lastHttpIndex-1] // 去掉最后的冒号
				healthURL := remaining[lastHttpIndex:]
				
				h.workers = append(h.workers, WorkerConfig{
					Name:      name,
					URL:       dataURL,
					HealthURL: healthURL,
				})
			}
		}
	}
}

// CheckWorkerHealth 检查单个 worker 健康状态
func (h *HealthChecker) CheckWorkerHealth(ctx context.Context, worker WorkerConfig) *WorkerHealthResult {
	start := time.Now()
	
	result := &WorkerHealthResult{
		Name:      worker.Name,
		URL:       worker.URL,
		CheckedAt: time.Now(),
	}

	// 基础健康检查
	req, err := http.NewRequestWithContext(ctx, "GET", worker.HealthURL, nil)
	if err != nil {
		result.Healthy = false
		result.Error = err.Error()
		result.ResponseTime = time.Since(start)
		return result
	}

	resp, err := h.client.Do(req)
	if err != nil {
		result.Healthy = false
		result.Error = err.Error()
		result.ResponseTime = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	result.ResponseTime = time.Since(start)
	result.Healthy = resp.StatusCode == http.StatusOK

	if !result.Healthy {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return result
	}

	// 获取详细指标信息
	h.enrichWithMetrics(ctx, worker, result)

	return result
}

// enrichWithMetrics 使用 Prometheus 指标丰富健康检查结果
func (h *HealthChecker) enrichWithMetrics(ctx context.Context, worker WorkerConfig, result *WorkerHealthResult) {
	// 构建 Prometheus 指标 URL
	metricsURL := h.buildMetricsURL(worker.HealthURL)
	if metricsURL == "" {
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", metricsURL, nil)
	if err != nil {
		return
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	// 解析 Prometheus 指标
	components, metrics := h.parsePrometheusMetrics(resp)
	result.Components = components
	result.Metrics = metrics
}

// buildMetricsURL 构建 Prometheus 指标 URL
func (h *HealthChecker) buildMetricsURL(healthURL string) string {
	// 将健康检查 URL 转换为指标 URL
	// 例如: http://host:18686/health -> http://host:9598/metrics
	if strings.Contains(healthURL, ":18686/health") {
		return strings.Replace(healthURL, ":18686/health", ":9598/metrics", 1)
	}
	if strings.Contains(healthURL, ":8686/health") {
		return strings.Replace(healthURL, ":8686/health", ":9598/metrics", 1)
	}
	return ""
}

// isBusinessComponent 判断是否为业务组件 (排除内部监控组件)
func (h *HealthChecker) isBusinessComponent(componentID string) bool {
	// 内部监控组件，不计入业务指标
	internalComponents := map[string]bool{
		"internal_metrics":   true,
		"prometheus_metrics": true,
	}
	
	return !internalComponents[componentID]
}

// parsePrometheusMetrics 解析 Prometheus 指标
func (h *HealthChecker) parsePrometheusMetrics(resp *http.Response) (*WorkerComponents, *WorkerMetrics) {
	scanner := bufio.NewScanner(resp.Body)
	
	componentMap := make(map[string]*ComponentStatus)
	metrics := &WorkerMetrics{
		DataSources: make(map[string]ComponentMetrics),
	}
	
	// 正则表达式匹配指标
	metricRegex := regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)\{([^}]*)\}\s+([0-9.]+)`)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		
		matches := metricRegex.FindStringSubmatch(line)
		if len(matches) != 4 {
			continue
		}
		
		metricName := matches[1]
		labels := matches[2]
		valueStr := matches[3]
		
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		
		// 解析标签
		labelMap := h.parseLabels(labels)
		componentID := labelMap["component_id"]
		componentType := labelMap["component_type"]
		componentKind := labelMap["component_kind"]
		
		// 处理组件相关指标
		if componentID != "" {
			// 只处理业务组件
			if h.isBusinessComponent(componentID) {
				if _, exists := componentMap[componentID]; !exists {
					componentMap[componentID] = &ComponentStatus{
						ID:     componentID,
						Type:   componentType,
						Kind:   componentKind,
						Status: "active",
					}
				}
				
				component := componentMap[componentID]
				
				// 根据指标名称更新组件状态
				switch {
				case strings.Contains(metricName, "received_events_total"):
					component.EventsTotal = int64(value)
					
					// 只有 source 组件才计入事件统计
					if componentKind == "source" {
						metrics.EventsReceived += int64(value)
					}
					
					// 记录数据源详情
					if _, exists := metrics.DataSources[componentID]; !exists {
						metrics.DataSources[componentID] = ComponentMetrics{}
					}
					ds := metrics.DataSources[componentID]
					ds.EventsReceived = int64(value)
					metrics.DataSources[componentID] = ds
					
				case strings.Contains(metricName, "received_bytes_total"):
					component.BytesTotal = int64(value)
					
					// 只有 source 组件才计入字节统计
					if componentKind == "source" {
						metrics.BytesReceived += int64(value)
					}
					
					// 记录数据源详情
					if _, exists := metrics.DataSources[componentID]; !exists {
						metrics.DataSources[componentID] = ComponentMetrics{}
					}
					ds := metrics.DataSources[componentID]
					ds.BytesReceived = int64(value)
					metrics.DataSources[componentID] = ds
					
				case strings.Contains(metricName, "errors_total"):
					component.ErrorsTotal = int64(value)
					// 累计所有业务组件的错误数
					metrics.ErrorsTotal += int64(value)
					
					// 记录数据源详情
					if _, exists := metrics.DataSources[componentID]; !exists {
						metrics.DataSources[componentID] = ComponentMetrics{}
					}
					ds := metrics.DataSources[componentID]
					ds.ErrorsTotal = int64(value)
					metrics.DataSources[componentID] = ds
				}
			}
		}
		
		// 处理缓冲区指标 (所有组件)
		if strings.Contains(metricName, "buffer_events") {
			// 计算缓冲区利用率 (简化版)
			if value > 0 {
				metrics.BufferUtilization += value
			}
		}
	}
	
	// 构建组件列表 (只包含业务组件)
	components := &WorkerComponents{
		Components: make([]ComponentStatus, 0, len(componentMap)),
	}
	
	for _, component := range componentMap {
		components.Components = append(components.Components, *component)
		components.Total++
		if component.ErrorsTotal == 0 {
			components.Active++
		} else {
			components.Failed++
		}
	}
	
	// 计算处理的事件数 (业务数据)
	metrics.EventsProcessed = metrics.EventsReceived
	metrics.BytesProcessed = metrics.BytesReceived
	
	return components, metrics
}

// parseLabels 解析 Prometheus 标签
func (h *HealthChecker) parseLabels(labelsStr string) map[string]string {
	labels := make(map[string]string)
	
	// 简单的标签解析 key="value"
	labelRegex := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)="([^"]*)"`)
	matches := labelRegex.FindAllStringSubmatch(labelsStr, -1)
	
	for _, match := range matches {
		if len(match) == 3 {
			labels[match[1]] = match[2]
		}
	}
	
	return labels
}

// CheckAllWorkers 检查所有 worker 健康状态
func (h *HealthChecker) CheckAllWorkers(ctx context.Context) []*WorkerHealthResult {
	results := make([]*WorkerHealthResult, len(h.workers))
	
	for i, worker := range h.workers {
		results[i] = h.CheckWorkerHealth(ctx, worker)
	}
	
	return results
}

// GetHealthyWorkers 获取健康的 worker 列表
func (h *HealthChecker) GetHealthyWorkers(ctx context.Context) []WorkerConfig {
	results := h.CheckAllWorkers(ctx)
	healthy := make([]WorkerConfig, 0)
	
	for i, result := range results {
		if result.Healthy {
			healthy = append(healthy, h.workers[i])
		}
	}
	
	return healthy
}

// GetWorkers 获取所有配置的 worker
func (h *HealthChecker) GetWorkers() []WorkerConfig {
	return h.workers
}

// SelectHealthyWorker 选择一个健康的 worker (简单轮询)
func (h *HealthChecker) SelectHealthyWorker(ctx context.Context) *WorkerConfig {
	healthy := h.GetHealthyWorkers(ctx)
	if len(healthy) == 0 {
		return nil
	}
	
	// 简单轮询选择
	return &healthy[0]
}

// OverallHealthStatus 整体健康状态
type OverallHealthStatus struct {
	Healthy          bool                  `json:"healthy"`
	TotalWorkers     int                   `json:"total_workers"`
	HealthyWorkers   int                   `json:"healthy_workers"`
	UnhealthyWorkers int                   `json:"unhealthy_workers"`
	Workers          []*WorkerHealthResult `json:"workers"`
	CheckedAt        time.Time             `json:"checked_at"`
}

// GetOverallStatus 获取整体健康状态
func (h *HealthChecker) GetOverallStatus(ctx context.Context) *OverallHealthStatus {
	results := h.CheckAllWorkers(ctx)
	
	status := &OverallHealthStatus{
		TotalWorkers:     len(results),
		HealthyWorkers:   0,
		UnhealthyWorkers: 0,
		Workers:          results,
		CheckedAt:        time.Now(),
	}
	
	for _, result := range results {
		if result.Healthy {
			status.HealthyWorkers++
		} else {
			status.UnhealthyWorkers++
		}
	}
	
	// 只要有一个 worker 健康，整体就是健康的
	status.Healthy = status.HealthyWorkers > 0
	
	return status
}

// SystemComponentHealth 系统组件健康状态
type SystemComponentHealth struct {
	Name         string        `json:"name"`
	Type         string        `json:"type"`
	Healthy      bool          `json:"healthy"`
	Status       string        `json:"status"`
	ResponseTime time.Duration `json:"response_time_ms"`
	Error        string        `json:"error,omitempty"`
	Details      interface{}   `json:"details,omitempty"`
	CheckedAt    time.Time     `json:"checked_at"`
}

// SystemHealthStatus 系统整体健康状态
type SystemHealthStatus struct {
	Healthy        bool                     `json:"healthy"`
	Status         string                   `json:"status"`
	Components     []*SystemComponentHealth `json:"components"`
	Workers        []*WorkerHealthResult    `json:"workers,omitempty"`
	Summary        SystemHealthSummary      `json:"summary"`
	CheckedAt      time.Time                `json:"checked_at"`
}

// SystemHealthSummary 系统健康摘要
type SystemHealthSummary struct {
	TotalComponents   int `json:"total_components"`
	HealthyComponents int `json:"healthy_components"`
	FailedComponents  int `json:"failed_components"`
	TotalWorkers      int `json:"total_workers"`
	HealthyWorkers    int `json:"healthy_workers"`
	UnhealthyWorkers  int `json:"unhealthy_workers"`
}

// GetSystemHealth 获取系统整体健康状态
func (h *HealthChecker) GetSystemHealth(ctx context.Context, db *sql.DB) *SystemHealthStatus {
	start := time.Now()
	
	var components []*SystemComponentHealth
	
	// 1. 检查数据库健康状态
	dbHealth := h.checkDatabaseHealth(ctx, db)
	components = append(components, dbHealth)
	
	// 2. 检查 OpenSearch 健康状态
	opensearchHealth := h.checkOpenSearchHealth(ctx)
	components = append(components, opensearchHealth)
	
	// 3. 检查 Kafka 健康状态
	kafkaHealth := h.checkKafkaHealth(ctx)
	components = append(components, kafkaHealth)
	
	// 4. 检查 Prometheus 健康状态
	prometheusHealth := h.checkPrometheusHealth(ctx)
	components = append(components, prometheusHealth)
	
	// 5. 检查 Vector 健康状态 (通过现有的 worker 检查)
	workerResults := h.CheckAllWorkers(ctx)
	
	// 计算摘要
	summary := SystemHealthSummary{
		TotalComponents:   len(components),
		HealthyComponents: 0,
		FailedComponents:  0,
		TotalWorkers:      len(workerResults),
		HealthyWorkers:    0,
		UnhealthyWorkers:  0,
	}
	
	systemHealthy := true
	
	// 统计组件健康状态
	for _, comp := range components {
		if comp.Healthy {
			summary.HealthyComponents++
		} else {
			summary.FailedComponents++
			systemHealthy = false
		}
	}
	
	// 统计 Worker 健康状态
	for _, worker := range workerResults {
		if worker.Healthy {
			summary.HealthyWorkers++
		} else {
			summary.UnhealthyWorkers++
		}
	}
	
	// 如果没有健康的 worker，系统也不健康
	if summary.HealthyWorkers == 0 && summary.TotalWorkers > 0 {
		systemHealthy = false
	}
	
	status := "healthy"
	if !systemHealthy {
		status = "unhealthy"
	}
	
	return &SystemHealthStatus{
		Healthy:    systemHealthy,
		Status:     status,
		Components: components,
		Workers:    workerResults,
		Summary:    summary,
		CheckedAt:  start,
	}
}

// checkDatabaseHealth 检查数据库健康状态
func (h *HealthChecker) checkDatabaseHealth(ctx context.Context, db *sql.DB) *SystemComponentHealth {
	start := time.Now()
	
	health := &SystemComponentHealth{
		Name:      "database",
		Type:      "postgresql",
		CheckedAt: time.Now(),
	}
	
	if db == nil {
		health.Healthy = false
		health.Status = "unavailable"
		health.Error = "Database connection not available"
		health.ResponseTime = time.Since(start)
		return health
	}
	
	// 执行简单的 ping 测试
	err := db.PingContext(ctx)
	health.ResponseTime = time.Since(start)
	
	if err != nil {
		health.Healthy = false
		health.Status = "error"
		health.Error = err.Error()
		return health
	}
	
	// 执行简单查询测试连接
	var result int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		health.Healthy = false
		health.Status = "query_failed"
		health.Error = err.Error()
		return health
	}
	
	health.Healthy = true
	health.Status = "connected"
	health.Details = map[string]interface{}{
		"ping_success": true,
		"query_test":   "passed",
	}
	
	return health
}

// checkOpenSearchHealth 检查 OpenSearch 健康状态
func (h *HealthChecker) checkOpenSearchHealth(ctx context.Context) *SystemComponentHealth {
	start := time.Now()
	
	health := &SystemComponentHealth{
		Name:      "opensearch",
		Type:      "indexer",
		CheckedAt: time.Now(),
	}
	
	// 从环境变量获取 OpenSearch URL 和认证信息
	opensearchURL := os.Getenv("OPENSEARCH_URL")
	if opensearchURL == "" {
		opensearchURL = "http://localhost:9200"
	}
	
	opensearchUser := os.Getenv("OPENSEARCH_USERNAME")
	opensearchPassword := os.Getenv("OPENSEARCH_PASSWORD")
	
	// 如果没有配置认证信息，使用默认的监控用户
	if opensearchUser == "" {
		opensearchUser = "sysarmor_monitor"
	}
	if opensearchPassword == "" {
		opensearchPassword = "sysarmor_monitor"
	}
	
	// 检查集群健康状态
	healthURL := opensearchURL + "/_cluster/health"
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		health.Healthy = false
		health.Status = "request_failed"
		health.Error = err.Error()
		health.ResponseTime = time.Since(start)
		return health
	}
	
	// 添加基本认证
	req.SetBasicAuth(opensearchUser, opensearchPassword)
	
	resp, err := h.client.Do(req)
	health.ResponseTime = time.Since(start)
	
	if err != nil {
		health.Healthy = false
		health.Status = "connection_failed"
		health.Error = err.Error()
		return health
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == http.StatusUnauthorized {
		health.Healthy = false
		health.Status = "authentication_failed"
		health.Error = "Invalid credentials for OpenSearch"
		return health
	}
	
	if resp.StatusCode != http.StatusOK {
		health.Healthy = false
		health.Status = "http_error"
		health.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return health
	}
	
	// 解析响应
	var clusterHealth map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&clusterHealth); err != nil {
		health.Healthy = false
		health.Status = "parse_failed"
		health.Error = err.Error()
		return health
	}
	
	clusterStatus, _ := clusterHealth["status"].(string)
	health.Healthy = clusterStatus == "green" || clusterStatus == "yellow"
	health.Status = clusterStatus
	health.Details = clusterHealth
	
	return health
}

// checkKafkaHealth 检查 Kafka 健康状态
func (h *HealthChecker) checkKafkaHealth(ctx context.Context) *SystemComponentHealth {
	start := time.Now()
	
	health := &SystemComponentHealth{
		Name:      "kafka",
		Type:      "message_queue",
		CheckedAt: time.Now(),
	}
	
	// 通过 Manager 自身的 Kafka API 检查
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:8080/api/v1/services/kafka/test-connection", nil)
	if err != nil {
		health.Healthy = false
		health.Status = "request_failed"
		health.Error = err.Error()
		health.ResponseTime = time.Since(start)
		return health
	}
	
	resp, err := h.client.Do(req)
	health.ResponseTime = time.Since(start)
	
	if err != nil {
		health.Healthy = false
		health.Status = "connection_failed"
		health.Error = err.Error()
		return health
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		health.Healthy = false
		health.Status = "http_error"
		health.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return health
	}
	
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		health.Healthy = false
		health.Status = "parse_failed"
		health.Error = err.Error()
		return health
	}
	
	success, successOk := result["success"].(bool)
	connected, connectedOk := result["connected"].(bool)
	
	// 检查字段是否存在且为 true
	health.Healthy = successOk && success && connectedOk && connected
	if health.Healthy {
		health.Status = "connected"
	} else {
		health.Status = "disconnected"
		if errorMsg, exists := result["error"].(string); exists {
			health.Error = errorMsg
		} else if !successOk {
			health.Error = "Missing 'success' field in response"
		} else if !connectedOk {
			health.Error = "Missing 'connected' field in response"
		} else if !success {
			health.Error = "Kafka connection test failed"
		} else if !connected {
			health.Error = "Kafka not connected"
		}
	}
	health.Details = result
	
	return health
}

// checkPrometheusHealth 检查 Prometheus 健康状态
func (h *HealthChecker) checkPrometheusHealth(ctx context.Context) *SystemComponentHealth {
	start := time.Now()
	
	health := &SystemComponentHealth{
		Name:      "prometheus",
		Type:      "monitoring",
		CheckedAt: time.Now(),
	}
	
	// 从环境变量获取 Prometheus URL
	prometheusURL := os.Getenv("PROMETHEUS_URL")
	if prometheusURL == "" {
		prometheusURL = "http://localhost:9090"
	}
	
	// 检查 Prometheus 健康状态
	healthURL := prometheusURL + "/-/healthy"
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		health.Healthy = false
		health.Status = "request_failed"
		health.Error = err.Error()
		health.ResponseTime = time.Since(start)
		return health
	}
	
	resp, err := h.client.Do(req)
	health.ResponseTime = time.Since(start)
	
	if err != nil {
		health.Healthy = false
		health.Status = "connection_failed"
		health.Error = err.Error()
		return health
	}
	defer resp.Body.Close()
	
	health.Healthy = resp.StatusCode == http.StatusOK
	if health.Healthy {
		health.Status = "healthy"
		
		// 获取额外信息
		configURL := prometheusURL + "/api/v1/status/config"
		if configReq, err := http.NewRequestWithContext(ctx, "GET", configURL, nil); err == nil {
			if configResp, err := h.client.Do(configReq); err == nil {
				defer configResp.Body.Close()
				if configResp.StatusCode == http.StatusOK {
					var config map[string]interface{}
					if json.NewDecoder(configResp.Body).Decode(&config) == nil {
						health.Details = map[string]interface{}{
							"status": "healthy",
							"config_loaded": true,
						}
					}
				}
			}
		}
	} else {
		health.Status = "unhealthy"
		health.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	
	return health
}
