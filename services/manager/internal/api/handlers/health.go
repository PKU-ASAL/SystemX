package handlers

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/services/manager/internal/services/health"
)

// HealthHandler 健康检查处理器
type HealthHandler struct {
	healthChecker *health.HealthChecker
	db            *sql.DB
}

// NewHealthHandler 创建健康检查处理器
func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{
		healthChecker: health.NewHealthChecker(),
		db:            db,
	}
}

// GetHealth 获取健康状态
// @Summary 获取系统健康状态
// @Description 获取所有 Worker 的健康状态和系统整体状态
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "健康状态正常"
// @Failure 503 {object} map[string]interface{} "没有健康的 Worker"
// @Router /health [get]
func (h *HealthHandler) GetHealth(c *gin.Context) {
	ctx := c.Request.Context()
	status := h.healthChecker.GetOverallStatus(ctx)

	if status.Healthy {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    status,
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    status,
			"message": "No healthy workers available",
		})
	}
}

// GetWorkers 获取所有 worker 状态
// @Summary 获取所有 Worker 状态
// @Description 获取所有已配置 Worker 的健康状态详情
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Worker 状态列表"
// @Router /health/workers [get]
func (h *HealthHandler) GetWorkers(c *gin.Context) {
	ctx := c.Request.Context()
	results := h.healthChecker.CheckAllWorkers(ctx)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
	})
}

// GetHealthyWorkers 获取健康的 worker 列表
func (h *HealthHandler) GetHealthyWorkers(c *gin.Context) {
	ctx := c.Request.Context()
	healthy := h.healthChecker.GetHealthyWorkers(ctx)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    healthy,
	})
}

// SelectWorker 选择一个健康的 worker
func (h *HealthHandler) SelectWorker(c *gin.Context) {
	ctx := c.Request.Context()
	worker := h.healthChecker.SelectHealthyWorker(ctx)

	if worker == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "No healthy workers available",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    worker,
	})
}

// GetWorkerDetails 获取特定 worker 的详细信息
func (h *HealthHandler) GetWorkerDetails(c *gin.Context) {
	workerName := c.Param("name")
	ctx := c.Request.Context()
	
	// 查找指定的 worker
	workers := h.healthChecker.GetWorkers()
	var targetWorker *health.WorkerConfig
	
	for _, worker := range workers {
		if worker.Name == workerName {
			targetWorker = &worker
			break
		}
	}
	
	if targetWorker == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Worker not found",
		})
		return
	}
	
	// 获取详细健康信息
	result := h.healthChecker.CheckWorkerHealth(ctx, *targetWorker)
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetWorkerMetrics 获取特定 worker 的指标信息
func (h *HealthHandler) GetWorkerMetrics(c *gin.Context) {
	workerName := c.Param("name")
	ctx := c.Request.Context()
	
	// 查找指定的 worker
	workers := h.healthChecker.GetWorkers()
	var targetWorker *health.WorkerConfig
	
	for _, worker := range workers {
		if worker.Name == workerName {
			targetWorker = &worker
			break
		}
	}
	
	if targetWorker == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Worker not found",
		})
		return
	}
	
	// 获取详细健康信息
	result := h.healthChecker.CheckWorkerHealth(ctx, *targetWorker)
	
	if !result.Healthy {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Worker is unhealthy",
			"details": result.Error,
		})
		return
	}
	
	// 只返回指标信息
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"worker":     workerName,
			"metrics":    result.Metrics,
			"components": result.Components,
			"checked_at": result.CheckedAt,
		},
	})
}

// GetWorkerComponents 获取特定 worker 的组件状态
func (h *HealthHandler) GetWorkerComponents(c *gin.Context) {
	workerName := c.Param("name")
	ctx := c.Request.Context()
	
	// 查找指定的 worker
	workers := h.healthChecker.GetWorkers()
	var targetWorker *health.WorkerConfig
	
	for _, worker := range workers {
		if worker.Name == workerName {
			targetWorker = &worker
			break
		}
	}
	
	if targetWorker == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Worker not found",
		})
		return
	}
	
	// 获取详细健康信息
	result := h.healthChecker.CheckWorkerHealth(ctx, *targetWorker)
	
	if !result.Healthy {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Worker is unhealthy",
			"details": result.Error,
		})
		return
	}
	
	// 只返回组件信息
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"worker":     workerName,
			"components": result.Components,
			"checked_at": result.CheckedAt,
		},
	})
}

// GetSystemHealth 获取系统整体健康状态摘要
func (h *HealthHandler) GetSystemHealth(c *gin.Context) {
	ctx := c.Request.Context()
	status := h.healthChecker.GetOverallStatus(ctx)
	
	// 计算汇总指标 (只统计 source 组件数据)
	var totalEvents, totalBytes, totalErrors int64
	var totalComponents int
	
	for _, worker := range status.Workers {
		if worker.Metrics != nil {
			totalEvents += worker.Metrics.EventsReceived
			totalBytes += worker.Metrics.BytesReceived
			totalErrors += worker.Metrics.ErrorsTotal
		}
		if worker.Components != nil {
			totalComponents += worker.Components.Total
		}
	}
	
	summary := gin.H{
		"system_healthy":    status.Healthy,
		"total_workers":     status.TotalWorkers,
		"healthy_workers":   status.HealthyWorkers,
		"unhealthy_workers": status.UnhealthyWorkers,
		"total_components":  totalComponents,
		"events_total":      totalEvents,
		"bytes_total":       totalBytes,
		"errors_total":      totalErrors,
		"checked_at":        status.CheckedAt,
	}
	
	if status.Healthy {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    summary,
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    summary,
			"message": "System is unhealthy",
		})
	}
}

// GetComprehensiveHealth 获取综合系统健康状态
// @Summary 获取综合系统健康状态
// @Description 获取包括数据库、OpenSearch、Kafka、Prometheus、Vector 等所有组件的综合健康状态
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} health.SystemHealthStatus "系统健康状态"
// @Failure 503 {object} map[string]interface{} "系统不健康"
// @Router /health/comprehensive [get]
func (h *HealthHandler) GetComprehensiveHealth(c *gin.Context) {
	ctx := c.Request.Context()
	systemHealth := h.healthChecker.GetSystemHealth(ctx, h.db)
	
	if systemHealth.Healthy {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    systemHealth,
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    systemHealth,
			"message": "System is unhealthy",
		})
	}
}

// GetHealthOverview 获取健康状态概览 (替代原有的 /health 接口)
// @Summary 获取健康状态概览
// @Description 获取系统健康状态概览，包括所有组件的简要状态信息
// @Tags health
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "健康状态概览"
// @Failure 503 {object} map[string]interface{} "系统不健康"
// @Router /health/overview [get]
func (h *HealthHandler) GetHealthOverview(c *gin.Context) {
	ctx := c.Request.Context()
	systemHealth := h.healthChecker.GetSystemHealth(ctx, h.db)
	
	// 构建概览响应
	overview := gin.H{
		"healthy":     systemHealth.Healthy,
		"status":      systemHealth.Status,
		"summary":     systemHealth.Summary,
		"checked_at":  systemHealth.CheckedAt,
	}
	
	// 添加组件状态摘要
	componentSummary := make(map[string]interface{})
	for _, comp := range systemHealth.Components {
		componentSummary[comp.Name] = gin.H{
			"healthy":       comp.Healthy,
			"status":        comp.Status,
			"response_time": comp.ResponseTime,
		}
	}
	overview["components"] = componentSummary
	
	// 添加 Worker 状态摘要
	if len(systemHealth.Workers) > 0 {
		workerSummary := make([]gin.H, 0, len(systemHealth.Workers))
		for _, worker := range systemHealth.Workers {
			workerSummary = append(workerSummary, gin.H{
				"name":          worker.Name,
				"healthy":       worker.Healthy,
				"response_time": worker.ResponseTime,
			})
		}
		overview["workers"] = workerSummary
	}
	
	if systemHealth.Healthy {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    overview,
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"data":    overview,
			"message": "System is unhealthy",
		})
	}
}
