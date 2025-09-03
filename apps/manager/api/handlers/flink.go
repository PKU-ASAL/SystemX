package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/apps/manager/services/flink"
)

// FlinkHandler Flink 管理处理器
type FlinkHandler struct {
	flinkService *flink.FlinkService
}

// NewFlinkHandler 创建 Flink 管理处理器
func NewFlinkHandler(flinkBaseURL string) *FlinkHandler {
	return &FlinkHandler{
		flinkService: flink.NewFlinkService(flinkBaseURL),
	}
}

// TestFlinkConnection 测试 Flink 连接
// @Summary 测试 Flink 连接
// @Description 测试与 Flink 集群的连接状态
// @Tags flink
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "连接成功"
// @Failure 500 {object} map[string]interface{} "连接失败"
// @Router /services/flink/test-connection [get]
func (h *FlinkHandler) TestFlinkConnection(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 尝试获取集群概览来测试连接
	overview, err := h.flinkService.GetClusterOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":   false,
			"connected": false,
			"error":     "Failed to connect to Flink: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"connected":    true,
		"message":      "Successfully connected to Flink",
		"cluster_info": overview,
		"tested_at":    time.Now(),
	})
}

// GetClusterOverview 获取 Flink 集群概览
// @Summary 获取 Flink 集群概览
// @Description 获取 Flink 集群的基本信息，包括状态、TaskManager 数量、作业数量等
// @Tags flink
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "集群概览信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/flink/overview [get]
func (h *FlinkHandler) GetClusterOverview(c *gin.Context) {
	ctx := c.Request.Context()
	
	overview, err := h.flinkService.GetClusterOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get cluster overview: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    overview,
	})
}

// GetJobs 获取 Flink 作业列表
// @Summary 获取 Flink 作业列表
// @Description 获取 Flink 集群中所有作业的信息
// @Tags flink
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "作业列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/flink/jobs [get]
func (h *FlinkHandler) GetJobs(c *gin.Context) {
	ctx := c.Request.Context()
	
	jobs, err := h.flinkService.GetJobs(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get jobs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    jobs,
	})
}

// GetJobsOverview 获取作业概览信息
// @Summary 获取作业概览信息
// @Description 获取 Flink 集群中所有作业的概览信息，包括状态统计、分类等
// @Tags flink
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "作业概览信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/flink/jobs/overview [get]
func (h *FlinkHandler) GetJobsOverview(c *gin.Context) {
	ctx := c.Request.Context()
	
	overview, err := h.flinkService.GetJobsOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get jobs overview: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    overview,
	})
}

// GetJobDetails 获取特定作业的详细信息
// @Summary 获取特定作业的详细信息
// @Description 获取指定作业的详细信息，包括顶点、状态、执行计划等
// @Tags flink
// @Accept json
// @Produce json
// @Param job_id path string true "作业 ID"
// @Success 200 {object} map[string]interface{} "作业详细信息"
// @Failure 404 {object} map[string]interface{} "作业不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/flink/jobs/{job_id} [get]
func (h *FlinkHandler) GetJobDetails(c *gin.Context) {
	ctx := c.Request.Context()
	jobID := c.Param("job_id")
	
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "job_id is required",
		})
		return
	}
	
	jobDetails, err := h.flinkService.GetJobDetails(ctx, jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get job details: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    jobDetails,
	})
}

// GetJobMetrics 获取作业指标
// @Summary 获取作业指标
// @Description 获取指定作业的性能指标信息
// @Tags flink
// @Accept json
// @Produce json
// @Param job_id path string true "作业 ID"
// @Success 200 {object} map[string]interface{} "作业指标信息"
// @Failure 404 {object} map[string]interface{} "作业不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/flink/jobs/{job_id}/metrics [get]
func (h *FlinkHandler) GetJobMetrics(c *gin.Context) {
	ctx := c.Request.Context()
	jobID := c.Param("job_id")
	
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "job_id is required",
		})
		return
	}
	
	metrics, err := h.flinkService.GetJobMetrics(ctx, jobID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get job metrics: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    metrics,
	})
}

// GetTaskManagers 获取 TaskManager 信息
// @Summary 获取 TaskManager 信息
// @Description 获取 Flink 集群中所有 TaskManager 的详细信息
// @Tags flink
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "TaskManager 信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/flink/taskmanagers [get]
func (h *FlinkHandler) GetTaskManagers(c *gin.Context) {
	ctx := c.Request.Context()
	
	taskManagers, err := h.flinkService.GetTaskManagers(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get task managers: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    taskManagers,
	})
}

// GetTaskManagersOverview 获取 TaskManager 概览信息
// @Summary 获取 TaskManager 概览信息
// @Description 获取 Flink 集群中所有 TaskManager 的概览信息，包括资源使用统计
// @Tags flink
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "TaskManager 概览信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/flink/taskmanagers/overview [get]
func (h *FlinkHandler) GetTaskManagersOverview(c *gin.Context) {
	ctx := c.Request.Context()
	
	overview, err := h.flinkService.GetTaskManagersOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get task managers overview: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    overview,
	})
}

// GetConfig 获取 Flink 配置
// @Summary 获取 Flink 配置
// @Description 获取 Flink 集群的配置信息
// @Tags flink
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Flink 配置信息"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/flink/config [get]
func (h *FlinkHandler) GetConfig(c *gin.Context) {
	ctx := c.Request.Context()
	
	config, err := h.flinkService.GetConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// GetClusterHealth 获取集群健康状态
// @Summary 获取集群健康状态
// @Description 获取 Flink 集群的健康状态，包括 TaskManager 健康度、作业状态等
// @Tags flink
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "集群健康状态"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /services/flink/health [get]
func (h *FlinkHandler) GetClusterHealth(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 获取集群概览
	overview, err := h.flinkService.GetClusterOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get cluster overview: " + err.Error(),
		})
		return
	}
	
	// 获取 TaskManager 概览
	tmOverview, err := h.flinkService.GetTaskManagersOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get task managers overview: " + err.Error(),
		})
		return
	}
	
	// 计算健康状态
	healthy := true
	var issues []string
	
	// 检查是否有失败的作业
	if overview.JobsFailed > 0 {
		healthy = false
		issues = append(issues, "有失败的作业")
	}
	
	// 检查 TaskManager 健康状态
	if tmOverview["unhealthy_taskmanagers"].(int) > 0 {
		healthy = false
		issues = append(issues, "有不健康的 TaskManager")
	}
	
	// 检查是否有可用的 slots
	if overview.SlotsAvailable == 0 && overview.JobsRunning > 0 {
		issues = append(issues, "没有可用的 slots")
	}
	
	status := "healthy"
	if !healthy {
		status = "unhealthy"
	} else if len(issues) > 0 {
		status = "warning"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"healthy":           healthy,
			"status":            status,
			"cluster_overview":  overview,
			"taskmanager_overview": tmOverview,
			"issues":            issues,
			"checked_at":        time.Now(),
		},
	})
}
