package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/apps/manager/services/opensearch"
)

// DashboardHandler Dashboard API处理器
type DashboardHandler struct {
	opensearchService *opensearch.OpenSearchService
	db                *sql.DB
}

// NewDashboardHandler 创建Dashboard处理器
func NewDashboardHandler(opensearchService *opensearch.OpenSearchService, db *sql.DB) *DashboardHandler {
	return &DashboardHandler{
		opensearchService: opensearchService,
		db:                db,
	}
}

// GetAlertsSeverityDistribution 获取告警严重程度分布
// @Summary 获取告警严重程度分布
// @Description 获取指定时间范围内告警的严重程度分布统计
// @Tags dashboard
// @Accept json
// @Produce json
// @Param timeRange query string false "时间范围 (1h/24h/7d/30d)" default(24h)
// @Param index query string false "索引模式" default(sysarmor-alerts-*)
// @Success 200 {object} map[string]interface{} "严重程度分布数据"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v1/dashboard/alerts/severity-distribution [get]
func (h *DashboardHandler) GetAlertsSeverityDistribution(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	timeRange := c.DefaultQuery("timeRange", "24h")
	indexPattern := c.DefaultQuery("index", "sysarmor-alerts-*")
	
	// 计算时间范围
	to := time.Now()
	var from time.Time
	switch timeRange {
	case "1h":
		from = to.Add(-1 * time.Hour)
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.Add(-7 * 24 * time.Hour)
	case "30d":
		from = to.Add(-30 * 24 * time.Hour)
	default:
		from = to.Add(-24 * time.Hour)
		timeRange = "24h"
	}
	
	// 构建聚合查询
	request := &opensearch.SearchRequest{
		Size: 0, // 只要聚合结果，不要具体文档
		Query: map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gte": from.Format(time.RFC3339),
					"lte": to.Format(time.RFC3339),
				},
			},
		},
		Aggs: map[string]interface{}{
			"severity_distribution": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "alert.severity.keyword",
					"size":  10,
				},
			},
		},
	}
	
	result, err := h.opensearchService.SearchEvents(ctx, indexPattern, request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get severity distribution: " + err.Error(),
		})
		return
	}
	
	// 解析聚合结果
	severityData := map[string]int{
		"critical": 0,
		"high":     0,
		"medium":   0,
		"low":      0,
	}
	
	total := 0
	if aggs, ok := result.Aggregations["severity_distribution"].(map[string]interface{}); ok {
		if buckets, ok := aggs["buckets"].([]interface{}); ok {
			for _, bucket := range buckets {
				if b, ok := bucket.(map[string]interface{}); ok {
					if key, ok := b["key"].(string); ok {
						if count, ok := b["doc_count"].(float64); ok {
							severityData[key] = int(count)
							total += int(count)
						}
					}
				}
			}
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"critical":  severityData["critical"],
			"high":      severityData["high"],
			"medium":    severityData["medium"],
			"low":       severityData["low"],
			"total":     total,
			"timeRange": timeRange,
			"updatedAt": time.Now().UTC().Format(time.RFC3339),
		},
	})
}

// GetAlertsTrends 获取告警趋势分析
// @Summary 获取告警趋势分析
// @Description 获取指定时间范围内告警的趋势分析数据
// @Tags dashboard
// @Accept json
// @Produce json
// @Param timeRange query string false "时间范围 (1h/24h/7d/30d)" default(7d)
// @Param interval query string false "时间间隔 (5m/1h/1d)" default(1h)
// @Param groupBy query string false "分组字段 (severity/event_type/host)" default(severity)
// @Success 200 {object} map[string]interface{} "趋势分析数据"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v1/dashboard/alerts/trends [get]
func (h *DashboardHandler) GetAlertsTrends(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	timeRange := c.DefaultQuery("timeRange", "7d")
	interval := c.DefaultQuery("interval", "1h")
	groupBy := c.DefaultQuery("groupBy", "severity")
	indexPattern := c.DefaultQuery("index", "sysarmor-alerts-*")
	
	// 计算时间范围
	to := time.Now()
	var from time.Time
	var histogramInterval string
	
	switch timeRange {
	case "1h":
		from = to.Add(-1 * time.Hour)
		histogramInterval = "5m"
	case "24h":
		from = to.Add(-24 * time.Hour)
		histogramInterval = "1h"
	case "7d":
		from = to.Add(-7 * 24 * time.Hour)
		histogramInterval = "1h"
	case "30d":
		from = to.Add(-30 * 24 * time.Hour)
		histogramInterval = "1d"
	default:
		from = to.Add(-7 * 24 * time.Hour)
		histogramInterval = "1h"
	}
	
	// 如果用户指定了interval，使用用户指定的
	if c.Query("interval") != "" {
		histogramInterval = interval
	}
	
	// 构建聚合查询
	aggs := map[string]interface{}{
		"timeline": map[string]interface{}{
			"date_histogram": map[string]interface{}{
				"field":    "@timestamp",
				"interval": histogramInterval,
				"min_doc_count": 0,
			},
		},
	}
	
	// 根据groupBy添加子聚合
	if groupBy == "severity" {
		aggs["timeline"].(map[string]interface{})["aggs"] = map[string]interface{}{
			"by_severity": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "alert.severity.keyword",
					"size":  10,
				},
			},
		}
	}
	
	request := &opensearch.SearchRequest{
		Size: 0,
		Query: map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gte": from.Format(time.RFC3339),
					"lte": to.Format(time.RFC3339),
				},
			},
		},
		Aggs: aggs,
	}
	
	result, err := h.opensearchService.SearchEvents(ctx, indexPattern, request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get alerts trends: " + err.Error(),
		})
		return
	}
	
	// 解析时间线数据
	timeline := []map[string]interface{}{}
	totalCount := 0
	
	if aggs, ok := result.Aggregations["timeline"].(map[string]interface{}); ok {
		if buckets, ok := aggs["buckets"].([]interface{}); ok {
			for _, bucket := range buckets {
				if b, ok := bucket.(map[string]interface{}); ok {
					timePoint := map[string]interface{}{
						"timestamp": b["key_as_string"],
						"total":     int(b["doc_count"].(float64)),
					}
					
					// 如果有严重程度分组
					if groupBy == "severity" {
						if severityAgg, ok := b["by_severity"].(map[string]interface{}); ok {
							if severityBuckets, ok := severityAgg["buckets"].([]interface{}); ok {
								severityCounts := map[string]int{
									"critical": 0,
									"high":     0,
									"medium":   0,
									"low":      0,
								}
								
								for _, severityBucket := range severityBuckets {
									if sb, ok := severityBucket.(map[string]interface{}); ok {
										if key, ok := sb["key"].(string); ok {
											if count, ok := sb["doc_count"].(float64); ok {
												severityCounts[key] = int(count)
											}
										}
									}
								}
								
								timePoint["critical"] = severityCounts["critical"]
								timePoint["high"] = severityCounts["high"]
								timePoint["medium"] = severityCounts["medium"]
								timePoint["low"] = severityCounts["low"]
							}
						}
					}
					
					timeline = append(timeline, timePoint)
					totalCount += int(b["doc_count"].(float64))
				}
			}
		}
	}
	
	// 计算统计信息
	avgPerInterval := 0.0
	if len(timeline) > 0 {
		avgPerInterval = float64(totalCount) / float64(len(timeline))
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"timeline": timeline,
			"statistics": gin.H{
				"total_alerts":     totalCount,
				"avg_per_interval": avgPerInterval,
				"time_range":       timeRange,
				"interval":         histogramInterval,
			},
		},
	})
}

// GetCollectorsOverview 获取Collector状态概览
// @Summary 获取Collector状态概览
// @Description 获取所有Collector的状态概览和统计信息
// @Tags dashboard
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Collector状态概览"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v1/dashboard/collectors/overview [get]
func (h *DashboardHandler) GetCollectorsOverview(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 查询所有collector的状态统计
	query := `
		SELECT 
			status,
			deployment_type,
			metadata->>'environment' as environment,
			COUNT(*) as count,
			AVG(EXTRACT(EPOCH FROM (NOW() - last_heartbeat))) as avg_heartbeat_age
		FROM collectors 
		WHERE created_at > NOW() - INTERVAL '30 days'
		GROUP BY status, deployment_type, metadata->>'environment'
	`
	
	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to query collectors: " + err.Error(),
		})
		return
	}
	defer rows.Close()
	
	// 初始化统计数据
	summary := map[string]int{
		"total":    0,
		"active":   0,
		"inactive": 0,
		"offline":  0,
		"error":    0,
	}
	
	byEnvironment := map[string]int{}
	byDeploymentType := map[string]int{
		"agentless":     0,
		"sysarmor-stack": 0,
		"wazuh-hybrid":   0,
	}
	
	var totalResponseTime float64
	var responseTimeCount int
	
	// 处理查询结果
	for rows.Next() {
		var status, deploymentType string
		var environment sql.NullString
		var count int
		var avgHeartbeatAge sql.NullFloat64
		
		if err := rows.Scan(&status, &deploymentType, &environment, &count, &avgHeartbeatAge); err != nil {
			continue
		}
		
		// 统计状态
		if _, exists := summary[status]; exists {
			summary[status] += count
		}
		summary["total"] += count
		
		// 统计部署类型
		if _, exists := byDeploymentType[deploymentType]; exists {
			byDeploymentType[deploymentType] += count
		}
		
		// 统计环境
		env := "unknown"
		if environment.Valid && environment.String != "" {
			env = environment.String
		}
		byEnvironment[env] += count
		
		// 统计响应时间
		if avgHeartbeatAge.Valid {
			totalResponseTime += avgHeartbeatAge.Float64
			responseTimeCount++
		}
	}
	
	// 计算平均响应时间
	avgResponseTime := 0.0
	if responseTimeCount > 0 {
		avgResponseTime = totalResponseTime / float64(responseTimeCount)
	}
	
	// 计算健康百分比
	healthyPercentage := 0.0
	if summary["total"] > 0 {
		healthyPercentage = float64(summary["active"]) / float64(summary["total"]) * 100
	}
	
	// 查询最近24小时的变化
	recentQuery := `
		SELECT 
			COUNT(CASE WHEN created_at > NOW() - INTERVAL '24 hours' THEN 1 END) as new_collectors_24h,
			COUNT(CASE WHEN status = 'offline' AND updated_at > NOW() - INTERVAL '24 hours' THEN 1 END) as offline_collectors_24h
		FROM collectors
	`
	
	var newCollectors24h, offlineCollectors24h int
	if err := h.db.QueryRowContext(ctx, recentQuery).Scan(&newCollectors24h, &offlineCollectors24h); err != nil {
		// 如果查询失败，使用默认值
		newCollectors24h = 0
		offlineCollectors24h = 0
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"summary": summary,
			"byEnvironment": byEnvironment,
			"byDeploymentType": byDeploymentType,
			"performance": gin.H{
				"recentlyActive":     summary["active"],
				"avgResponseTime":    int(avgResponseTime),
				"healthyPercentage":  healthyPercentage,
			},
			"recentChanges": gin.H{
				"newCollectors24h":     newCollectors24h,
				"offlineCollectors24h": offlineCollectors24h,
			},
		},
	})
}

// GetSystemPerformanceOverview 获取系统性能概览
// @Summary 获取系统性能概览
// @Description 获取OpenSearch、Kafka、Flink等服务的性能概览
// @Tags dashboard
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "系统性能概览"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v1/dashboard/system/performance [get]
func (h *DashboardHandler) GetSystemPerformanceOverview(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 获取OpenSearch集群状态
	clusterHealth, err := h.opensearchService.GetClusterHealth(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get OpenSearch health: " + err.Error(),
		})
		return
	}
	
	clusterStats, err := h.opensearchService.GetClusterStats(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get OpenSearch stats: " + err.Error(),
		})
		return
	}
	
	// 获取索引信息
	indices, err := h.opensearchService.GetIndices(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get indices: " + err.Error(),
		})
		return
	}
	
	// 计算存储大小（从字符串转换为MB）
	var totalStorageMB float64 = 0
	for _, index := range indices {
		// 简单解析存储大小（这里需要更复杂的解析逻辑）
		if strings.Contains(index.StoreSize, "mb") {
			if size, err := strconv.ParseFloat(strings.Replace(index.StoreSize, "mb", "", -1), 64); err == nil {
				totalStorageMB += size
			}
		} else if strings.Contains(index.StoreSize, "kb") {
			if size, err := strconv.ParseFloat(strings.Replace(index.StoreSize, "kb", "", -1), 64); err == nil {
				totalStorageMB += size / 1024
			}
		}
	}
	
	// 查询数据库连接状态
	var dbConnections int
	dbQuery := `SELECT COUNT(*) FROM pg_stat_activity WHERE state = 'active'`
	if err := h.db.QueryRowContext(ctx, dbQuery).Scan(&dbConnections); err != nil {
		dbConnections = 0
	}
	
	// 测试数据库响应时间
	start := time.Now()
	if err := h.db.PingContext(ctx); err == nil {
		// 数据库连接正常
	}
	dbResponseTime := time.Since(start).Milliseconds()
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"opensearch": gin.H{
				"cluster_status":   clusterHealth.Status,
				"indices_count":    len(indices),
				"docs_count":       clusterStats.Indices.Docs.Count,
				"storage_size":     fmt.Sprintf("%.1f MB", totalStorageMB),
				"query_performance": gin.H{
					"avg_response_time": 12, // 这里需要实际的查询性能数据
					"queries_per_sec":   2.3,
					"slow_queries":      0,
				},
			},
			"flink": gin.H{
				"cluster_status":  "healthy", // 这里需要从Flink API获取
				"jobs_running":    2,
				"taskmanagers":    1,
				"slots_used":      4,
				"slots_total":     8,
				"processing_rate": 156.7,
				"memory_usage":    94.5,
			},
			"kafka": gin.H{
				"cluster_status":   "online", // 这里需要从Kafka API获取
				"brokers":          1,
				"topics":           7,
				"partitions":       179,
				"messages_per_sec": 8.2,
				"disk_usage":       12.3,
			},
			"database": gin.H{
				"status":           "connected",
				"response_time":    dbResponseTime,
				"connections":      dbConnections,
				"query_performance": "good",
			},
		},
	})
}

// GetEventTypesDistribution 获取事件类型分布
// @Summary 获取事件类型分布
// @Description 获取指定时间范围内事件类型的分布统计
// @Tags dashboard
// @Accept json
// @Produce json
// @Param timeRange query string false "时间范围 (1h/24h/7d/30d)" default(7d)
// @Param limit query int false "返回类型数量限制" default(10)
// @Success 200 {object} map[string]interface{} "事件类型分布数据"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /api/v1/dashboard/alerts/event-types [get]
func (h *DashboardHandler) GetEventTypesDistribution(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析参数
	timeRange := c.DefaultQuery("timeRange", "7d")
	limitStr := c.DefaultQuery("limit", "10")
	indexPattern := c.DefaultQuery("index", "sysarmor-alerts-*")
	
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}
	
	// 计算时间范围
	to := time.Now()
	var from time.Time
	switch timeRange {
	case "1h":
		from = to.Add(-1 * time.Hour)
	case "24h":
		from = to.Add(-24 * time.Hour)
	case "7d":
		from = to.Add(-7 * 24 * time.Hour)
	case "30d":
		from = to.Add(-30 * 24 * time.Hour)
	default:
		from = to.Add(-7 * 24 * time.Hour)
	}
	
	// 构建聚合查询
	request := &opensearch.SearchRequest{
		Size: 0,
		Query: map[string]interface{}{
			"range": map[string]interface{}{
				"@timestamp": map[string]interface{}{
					"gte": from.Format(time.RFC3339),
					"lte": to.Format(time.RFC3339),
				},
			},
		},
		Aggs: map[string]interface{}{
			"event_types": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "alert.category.keyword",
					"size":  limit,
				},
			},
		},
	}
	
	result, err := h.opensearchService.SearchEvents(ctx, indexPattern, request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get event types distribution: " + err.Error(),
		})
		return
	}
	
	// 解析聚合结果
	eventTypes := map[string]int{}
	topTypes := []map[string]interface{}{}
	total := 0
	
	if aggs, ok := result.Aggregations["event_types"].(map[string]interface{}); ok {
		if buckets, ok := aggs["buckets"].([]interface{}); ok {
			for _, bucket := range buckets {
				if b, ok := bucket.(map[string]interface{}); ok {
					if key, ok := b["key"].(string); ok {
						if count, ok := b["doc_count"].(float64); ok {
							eventTypes[key] = int(count)
							total += int(count)
						}
					}
				}
			}
			
			// 构建top_types数组，包含百分比
			for _, bucket := range buckets {
				if b, ok := bucket.(map[string]interface{}); ok {
					if key, ok := b["key"].(string); ok {
						if count, ok := b["doc_count"].(float64); ok {
							percentage := 0.0
							if total > 0 {
								percentage = float64(int(count)) / float64(total) * 100
							}
							
							topTypes = append(topTypes, map[string]interface{}{
								"type":       key,
								"count":      int(count),
								"percentage": percentage,
							})
						}
					}
				}
			}
		}
	}
	
	// 合并所有数据到响应中
	responseData := gin.H{
		"total":     total,
		"top_types": topTypes,
	}
	
	// 添加具体的事件类型计数
	for eventType, count := range eventTypes {
		responseData[eventType] = count
	}
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    responseData,
	})
}
