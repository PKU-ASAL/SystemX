package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/apps/manager/models"
	"github.com/sysarmor/sysarmor/apps/manager/services/wazuh"
)

// WazuhHandler Wazuh API处理器
type WazuhHandler struct {
	wazuhService *wazuh.WazuhService
}

// NewWazuhHandler 创建新的Wazuh处理器
func NewWazuhHandler(wazuhService *wazuh.WazuhService) *WazuhHandler {
	return &WazuhHandler{
		wazuhService: wazuhService,
	}
}

// handleWazuhError 智能处理Wazuh错误，返回合适的HTTP状态码
func (h *WazuhHandler) handleWazuhError(c *gin.Context, err error, operation string) {
	errMsg := err.Error()
	
	// 根据错误类型返回不同的状态码
	switch {
	case strings.Contains(errMsg, "wazuh service is disabled"):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Wazuh service is currently disabled",
			"code":    "SERVICE_DISABLED",
			"message": "Please configure and enable Wazuh integration first",
		})
	case strings.Contains(errMsg, "not available"):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Wazuh service is not available",
			"code":    "SERVICE_UNAVAILABLE",
			"message": "Wazuh Manager or Indexer is not properly configured",
		})
	case strings.Contains(errMsg, "not found"):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   errMsg,
			"code":    "NOT_FOUND",
		})
	case strings.Contains(errMsg, "not yet implemented"):
		c.JSON(http.StatusNotImplemented, gin.H{
			"success": false,
			"error":   errMsg,
			"code":    "NOT_IMPLEMENTED",
			"message": "This feature is planned for future releases",
		})
	case strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "authentication"):
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Authentication failed",
			"code":    "AUTH_FAILED",
			"message": "Please check Wazuh credentials",
		})
	case strings.Contains(errMsg, "timeout"):
		c.JSON(http.StatusRequestTimeout, gin.H{
			"success": false,
			"error":   "Request timeout",
			"code":    "TIMEOUT",
			"message": "Wazuh service did not respond in time",
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   operation + ": " + errMsg,
			"code":    "INTERNAL_ERROR",
		})
	}
}

// RegisterRoutes 注册Wazuh相关路由
func (h *WazuhHandler) RegisterRoutes(router *gin.RouterGroup) {
	wazuh := router.Group("/wazuh")
	{
		// 配置管理
		config := wazuh.Group("/config")
		{
			config.GET("", h.GetConfig)
			config.PUT("", h.UpdateConfig)
			config.POST("/validate", h.ValidateConfig)
			config.POST("/reload", h.ReloadConfig)
		}

		// Manager API
		manager := wazuh.Group("/manager")
		{
			manager.GET("/info", h.GetManagerInfo)
			manager.GET("/status", h.GetManagerStatus)
			manager.GET("/logs", h.GetManagerLogs)
			manager.GET("/stats", h.GetManagerStats)
			manager.POST("/restart", h.RestartManager)
			manager.GET("/configuration", h.GetManagerConfiguration)
			manager.PUT("/configuration", h.UpdateManagerConfiguration)
		}

		// Agent管理
		agents := wazuh.Group("/agents")
		{
			agents.GET("", h.GetAgents)
			agents.POST("", h.AddAgent)
			agents.GET("/:id", h.GetAgent)
			agents.PUT("/:id", h.UpdateAgent)
			agents.DELETE("/:id", h.DeleteAgent)
			agents.POST("/:id/restart", h.RestartAgent)
			agents.GET("/:id/key", h.GetAgentKey)
			agents.POST("/:id/upgrade", h.UpgradeAgent)
			agents.GET("/:id/config", h.GetAgentConfig)
			agents.PUT("/:id/config", h.UpdateAgentConfig)
		}

		// 组管理
		groups := wazuh.Group("/groups")
		{
			groups.GET("", h.GetGroups)
			groups.POST("", h.CreateGroup)
			groups.GET("/:name", h.GetGroup)
			groups.PUT("/:name", h.UpdateGroup)
			groups.DELETE("/:name", h.DeleteGroup)
			groups.GET("/:name/agents", h.GetGroupAgents)
			groups.POST("/:name/agents", h.AddAgentToGroup)
			groups.DELETE("/:name/agents/:agent_id", h.RemoveAgentFromGroup)
			groups.GET("/:name/configuration", h.GetGroupConfiguration)
			groups.PUT("/:name/configuration", h.UpdateGroupConfiguration)
		}

		// 规则管理
		rules := wazuh.Group("/rules")
		{
			rules.GET("", h.GetRules)
			rules.GET("/:id", h.GetRule)
			rules.POST("", h.CreateRule)
			rules.PUT("/:id", h.UpdateRule)
			rules.DELETE("/:id", h.DeleteRule)
			rules.GET("/files", h.GetRuleFiles)
			rules.GET("/files/:filename", h.GetRuleFile)
			rules.PUT("/files/:filename", h.UpdateRuleFile)
		}

		// 解码器管理
		decoders := wazuh.Group("/decoders")
		{
			decoders.GET("", h.GetDecoders)
			decoders.GET("/:name", h.GetDecoder)
			decoders.POST("", h.CreateDecoder)
			decoders.PUT("/:name", h.UpdateDecoder)
			decoders.DELETE("/:name", h.DeleteDecoder)
			decoders.GET("/files", h.GetDecoderFiles)
			decoders.GET("/files/:filename", h.GetDecoderFile)
			decoders.PUT("/files/:filename", h.UpdateDecoderFile)
		}

		// CDB列表管理
		lists := wazuh.Group("/lists")
		{
			lists.GET("", h.GetLists)
			lists.GET("/:filename", h.GetList)
			lists.POST("", h.CreateList)
			lists.PUT("/:filename", h.UpdateList)
			lists.DELETE("/:filename", h.DeleteList)
		}

		// Indexer API
		indexer := wazuh.Group("/indexer")
		{
			indexer.GET("/health", h.GetIndexerHealth)
			indexer.GET("/info", h.GetIndexerInfo)
			indexer.GET("/indices", h.GetIndices)
			indexer.POST("/indices", h.CreateIndex)
			indexer.DELETE("/indices/:name", h.DeleteIndex)
			indexer.GET("/templates", h.GetIndexTemplates)
			indexer.POST("/templates", h.CreateIndexTemplate)
		}

		// 告警查询
		alerts := wazuh.Group("/alerts")
		{
			alerts.POST("/search", h.SearchAlerts)
			alerts.GET("/agent/:id", h.GetAlertsByAgent)
			alerts.GET("/rule/:id", h.GetAlertsByRule)
			alerts.GET("/level/:level", h.GetAlertsByLevel)
			alerts.POST("/aggregate", h.AggregateAlerts)
			alerts.GET("/stats", h.GetAlertStats)
		}

		// 监控和统计
		monitoring := wazuh.Group("/monitoring")
		{
			monitoring.GET("/overview", h.GetMonitoringOverview)
			monitoring.GET("/agents/summary", h.GetAgentsSummary)
			monitoring.GET("/alerts/summary", h.GetAlertsSummary)
			monitoring.GET("/system/stats", h.GetSystemStats)
		}
	}
}

// GetConfig 获取Wazuh配置
// @Summary 获取Wazuh配置
// @Description 获取当前的Wazuh认证配置信息（脱敏）
// @Tags Wazuh
// @Accept json
// @Produce json
// @Success 200 {object} models.WazuhConfigResponse
// @Failure 500 {object} map[string]interface{}
// @Router /wazuh/config [get]
func (h *WazuhHandler) GetConfig(c *gin.Context) {
	config := h.wazuhService.GetConfig()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// UpdateConfig 更新Wazuh配置
// @Summary 更新Wazuh配置
// @Description 更新Wazuh认证配置信息
// @Tags Wazuh
// @Accept json
// @Produce json
// @Param config body map[string]interface{} true "配置信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /wazuh/config [put]
func (h *WazuhHandler) UpdateConfig(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateConfig(ctx, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration updated successfully",
	})
}

// ValidateConfig 验证Wazuh配置
func (h *WazuhHandler) ValidateConfig(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.ValidateConfig(ctx, req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Configuration validation failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration is valid",
	})
}

// ReloadConfig 重新加载Wazuh配置
func (h *WazuhHandler) ReloadConfig(c *gin.Context) {
	ctx := context.Background()
	if err := h.wazuhService.ReloadConfig(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to reload config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Configuration reloaded successfully",
	})
}

// GetManagerInfo 获取Manager信息
// @Summary 获取Wazuh Manager信息
// @Description 获取Wazuh Manager的基本信息和状态
// @Tags Wazuh Manager
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{} "服务不可用"
// @Failure 500 {object} map[string]interface{} "内部错误"
// @Router /wazuh/manager/info [get]
func (h *WazuhHandler) GetManagerInfo(c *gin.Context) {
	ctx := context.Background()
	info, err := h.wazuhService.GetManagerInfo(ctx)
	if err != nil {
		h.handleWazuhError(c, err, "Failed to get manager info")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    info,
	})
}

// GetManagerStatus 获取Manager状态
// @Summary 获取Wazuh Manager状态
// @Description 获取Wazuh Manager的运行状态信息
// @Tags Wazuh Manager
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /wazuh/manager/status [get]
func (h *WazuhHandler) GetManagerStatus(c *gin.Context) {
	ctx := context.Background()
	status, err := h.wazuhService.GetManagerStatus(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get manager status: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}

// GetManagerLogs 获取Manager日志
func (h *WazuhHandler) GetManagerLogs(c *gin.Context) {
	ctx := context.Background()
	
	// 解析查询参数
	offset := c.DefaultQuery("offset", "0")
	limit := c.DefaultQuery("limit", "100")
	level := c.Query("level")
	search := c.Query("search")

	logs, err := h.wazuhService.GetManagerLogs(ctx, offset, limit, level, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get manager logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
	})
}

// GetManagerStats 获取Manager统计信息
func (h *WazuhHandler) GetManagerStats(c *gin.Context) {
	ctx := context.Background()
	stats, err := h.wazuhService.GetManagerStats(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get manager stats: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// RestartManager 重启Manager
func (h *WazuhHandler) RestartManager(c *gin.Context) {
	ctx := context.Background()
	if err := h.wazuhService.RestartManager(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to restart manager: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Manager restart initiated",
	})
}

// GetManagerConfiguration 获取Manager配置
func (h *WazuhHandler) GetManagerConfiguration(c *gin.Context) {
	ctx := context.Background()
	section := c.Query("section")
	
	config, err := h.wazuhService.GetManagerConfiguration(ctx, section)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get manager configuration: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// UpdateManagerConfiguration 更新Manager配置
func (h *WazuhHandler) UpdateManagerConfiguration(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateManagerConfiguration(ctx, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update manager configuration: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Manager configuration updated successfully",
	})
}

// GetAgents 获取Agent列表
// @Summary 获取Wazuh代理列表
// @Description 获取所有Wazuh代理的列表，支持分页和过滤
// @Tags Wazuh Manager
// @Accept json
// @Produce json
// @Param offset query int false "偏移量"
// @Param limit query int false "限制数量"
// @Param sort query string false "排序字段"
// @Param search query string false "搜索关键词"
// @Param status query string false "代理状态"
// @Success 200 {object} models.WazuhAgentListResponse
// @Failure 503 {object} map[string]interface{} "服务不可用"
// @Failure 500 {object} map[string]interface{} "内部错误"
// @Router /wazuh/agents [get]
func (h *WazuhHandler) GetAgents(c *gin.Context) {
	ctx := context.Background()
	
	// 解析查询参数
	offset := c.DefaultQuery("offset", "0")
	limit := c.DefaultQuery("limit", "100")
	sort := c.Query("sort")
	search := c.Query("search")
	status := c.Query("status")

	agents, err := h.wazuhService.GetAgents(ctx, offset, limit, sort, search, status)
	if err != nil {
		h.handleWazuhError(c, err, "Failed to get agents")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    agents,
	})
}

// AddAgent 添加Agent
// @Summary 添加Wazuh代理
// @Description 向Wazuh Manager添加新的代理
// @Tags Wazuh Manager
// @Accept json
// @Produce json
// @Param agent body models.WazuhAgent true "代理信息"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /wazuh/agents [post]
func (h *WazuhHandler) AddAgent(c *gin.Context) {
	var req models.WazuhAgent
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	agent, err := h.wazuhService.AddAgent(ctx, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to add agent: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    agent,
	})
}

// GetAgent 获取单个Agent
// @Summary 获取指定代理信息
// @Description 根据代理ID获取代理的详细信息
// @Tags Wazuh Manager
// @Accept json
// @Produce json
// @Param id path string true "代理ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /wazuh/agents/{id} [get]
func (h *WazuhHandler) GetAgent(c *gin.Context) {
	agentID := c.Param("id")
	
	ctx := context.Background()
	agent, err := h.wazuhService.GetAgent(ctx, agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Agent not found: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    agent,
	})
}

// UpdateAgent 更新Agent
func (h *WazuhHandler) UpdateAgent(c *gin.Context) {
	agentID := c.Param("id")
	
	var req models.WazuhAgent
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateAgent(ctx, agentID, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update agent: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent updated successfully",
	})
}

// DeleteAgent 删除Agent
// @Summary 删除Wazuh代理
// @Description 从Wazuh Manager删除指定的代理
// @Tags Wazuh Manager
// @Accept json
// @Produce json
// @Param id path string true "代理ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /wazuh/agents/{id} [delete]
func (h *WazuhHandler) DeleteAgent(c *gin.Context) {
	agentID := c.Param("id")
	
	ctx := context.Background()
	if err := h.wazuhService.DeleteAgent(ctx, agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete agent: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent deleted successfully",
	})
}

// RestartAgent 重启Agent
func (h *WazuhHandler) RestartAgent(c *gin.Context) {
	agentID := c.Param("id")
	
	ctx := context.Background()
	if err := h.wazuhService.RestartAgent(ctx, agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to restart agent: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent restart initiated",
	})
}

// GetAgentKey 获取Agent密钥
func (h *WazuhHandler) GetAgentKey(c *gin.Context) {
	agentID := c.Param("id")
	
	ctx := context.Background()
	key, err := h.wazuhService.GetAgentKey(ctx, agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get agent key: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"key": key},
	})
}

// UpgradeAgent 升级Agent
func (h *WazuhHandler) UpgradeAgent(c *gin.Context) {
	agentID := c.Param("id")
	
	var req struct {
		Version string `json:"version"`
		Force   bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpgradeAgent(ctx, agentID, req.Version, req.Force); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to upgrade agent: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent upgrade initiated",
	})
}

// GetAgentConfig 获取Agent配置
func (h *WazuhHandler) GetAgentConfig(c *gin.Context) {
	agentID := c.Param("id")
	section := c.Query("section")
	
	ctx := context.Background()
	config, err := h.wazuhService.GetAgentConfig(ctx, agentID, section)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get agent config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// UpdateAgentConfig 更新Agent配置
func (h *WazuhHandler) UpdateAgentConfig(c *gin.Context) {
	agentID := c.Param("id")
	
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateAgentConfig(ctx, agentID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update agent config: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent configuration updated successfully",
	})
}

// GetIndexerHealth 获取Indexer健康状态
// @Summary 获取Wazuh Indexer健康状态
// @Description 获取Wazuh Indexer的健康状态信息
// @Tags Wazuh Indexer
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{} "服务不可用"
// @Failure 500 {object} map[string]interface{} "内部错误"
// @Router /wazuh/indexer/health [get]
func (h *WazuhHandler) GetIndexerHealth(c *gin.Context) {
	ctx := context.Background()
	health, err := h.wazuhService.GetIndexerHealth(ctx)
	if err != nil {
		h.handleWazuhError(c, err, "Failed to get indexer health")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    health,
	})
}

// GetIndexerInfo 获取Indexer信息
func (h *WazuhHandler) GetIndexerInfo(c *gin.Context) {
	ctx := context.Background()
	info, err := h.wazuhService.GetIndexerInfo(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get indexer info: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    info,
	})
}

// GetIndices 获取索引列表
// @Summary 获取Wazuh索引列表
// @Description 获取Wazuh Indexer中的所有索引
// @Tags Wazuh Indexer
// @Accept json
// @Produce json
// @Param pattern query string false "索引模式"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /wazuh/indexer/indices [get]
func (h *WazuhHandler) GetIndices(c *gin.Context) {
	pattern := c.Query("pattern")
	
	ctx := context.Background()
	indices, err := h.wazuhService.GetIndices(ctx, pattern)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get indices: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    indices,
	})
}

// CreateIndex 创建索引
func (h *WazuhHandler) CreateIndex(c *gin.Context) {
	var req struct {
		Name     string                 `json:"name" binding:"required"`
		Settings map[string]interface{} `json:"settings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.CreateIndex(ctx, req.Name, req.Settings); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create index: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Index created successfully",
	})
}

// DeleteIndex 删除索引
func (h *WazuhHandler) DeleteIndex(c *gin.Context) {
	indexName := c.Param("name")
	
	ctx := context.Background()
	if err := h.wazuhService.DeleteIndex(ctx, indexName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete index: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Index deleted successfully",
	})
}

// SearchAlerts 搜索告警
// @Summary 搜索Wazuh告警
// @Description 根据查询条件搜索Wazuh告警信息
// @Tags Wazuh Alerts
// @Accept json
// @Produce json
// @Param query body models.WazuhSearchQuery true "搜索查询"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 503 {object} map[string]interface{} "服务不可用"
// @Failure 500 {object} map[string]interface{} "内部错误"
// @Router /wazuh/alerts/search [post]
func (h *WazuhHandler) SearchAlerts(c *gin.Context) {
	var req models.WazuhSearchQuery
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	result, err := h.wazuhService.SearchAlerts(ctx, &req)
	if err != nil {
		h.handleWazuhError(c, err, "Failed to search alerts")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetAlertsByAgent 根据Agent获取告警
func (h *WazuhHandler) GetAlertsByAgent(c *gin.Context) {
	agentID := c.Param("id")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	ctx := context.Background()
	alerts, err := h.wazuhService.GetAlertsByAgent(ctx, agentID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get alerts by agent: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    alerts,
	})
}

// GetAlertsByRule 根据规则获取告警
func (h *WazuhHandler) GetAlertsByRule(c *gin.Context) {
	ruleID := c.Param("id")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	ctx := context.Background()
	alerts, err := h.wazuhService.GetAlertsByRule(ctx, ruleID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get alerts by rule: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    alerts,
	})
}

// GetAlertsByLevel 根据级别获取告警
func (h *WazuhHandler) GetAlertsByLevel(c *gin.Context) {
	levelStr := c.Param("level")
	level, err := strconv.Atoi(levelStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid level parameter",
		})
		return
	}

	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	ctx := context.Background()
	alerts, err := h.wazuhService.GetAlertsByLevel(ctx, level, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get alerts by level: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    alerts,
	})
}

// AggregateAlerts 聚合告警统计
func (h *WazuhHandler) AggregateAlerts(c *gin.Context) {
	var req struct {
		Type  string `json:"type" binding:"required"`
		Field string `json:"field" binding:"required"`
		Size  int    `json:"size"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	if req.Size <= 0 {
		req.Size = 10
	}

	ctx := context.Background()
	result, err := h.wazuhService.AggregateAlerts(ctx, req.Type, req.Field, req.Size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to aggregate alerts: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// GetAlertStats 获取告警统计
// @Summary 获取告警统计信息
// @Description 获取Wazuh告警的统计信息
// @Tags Wazuh Alerts
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /wazuh/alerts/stats [get]
func (h *WazuhHandler) GetAlertStats(c *gin.Context) {
	ctx := context.Background()
	stats, err := h.wazuhService.GetAlertStats(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get alert stats: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetMonitoringOverview 获取监控概览
func (h *WazuhHandler) GetMonitoringOverview(c *gin.Context) {
	ctx := context.Background()
	overview, err := h.wazuhService.GetMonitoringOverview(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get monitoring overview: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    overview,
	})
}

// GetAgentsSummary 获取Agent摘要
func (h *WazuhHandler) GetAgentsSummary(c *gin.Context) {
	ctx := context.Background()
	summary, err := h.wazuhService.GetAgentsSummary(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get agents summary: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
	})
}

// GetAlertsSummary 获取告警摘要
func (h *WazuhHandler) GetAlertsSummary(c *gin.Context) {
	ctx := context.Background()
	summary, err := h.wazuhService.GetAlertsSummary(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get alerts summary: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    summary,
	})
}

// GetSystemStats 获取系统统计
func (h *WazuhHandler) GetSystemStats(c *gin.Context) {
	ctx := context.Background()
	stats, err := h.wazuhService.GetSystemStats(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get system stats: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    stats,
	})
}

// GetGroups 获取组列表
// @Summary 获取Wazuh组列表
// @Description 获取所有Wazuh代理组的列表，支持分页和过滤
// @Tags Wazuh Groups
// @Accept json
// @Produce json
// @Param offset query int false "偏移量"
// @Param limit query int false "限制数量"
// @Param sort query string false "排序字段"
// @Param search query string false "搜索关键词"
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{} "服务不可用"
// @Failure 500 {object} map[string]interface{} "内部错误"
// @Router /wazuh/groups [get]
func (h *WazuhHandler) GetGroups(c *gin.Context) {
	ctx := context.Background()
	
	offset := c.DefaultQuery("offset", "0")
	limit := c.DefaultQuery("limit", "100")
	sort := c.Query("sort")
	search := c.Query("search")

	groups, err := h.wazuhService.GetGroups(ctx, offset, limit, sort, search)
	if err != nil {
		h.handleWazuhError(c, err, "Failed to get groups")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    groups,
	})
}

// CreateGroup 创建组
// @Summary 创建Wazuh组
// @Description 创建新的Wazuh代理组
// @Tags Wazuh Groups
// @Accept json
// @Produce json
// @Param group body object{name=string} true "组信息"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /wazuh/groups [post]
func (h *WazuhHandler) CreateGroup(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.CreateGroup(ctx, req.Name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create group: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Group created successfully",
	})
}

// GetGroup 获取单个组
func (h *WazuhHandler) GetGroup(c *gin.Context) {
	groupName := c.Param("name")
	
	ctx := context.Background()
	group, err := h.wazuhService.GetGroup(ctx, groupName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Group not found: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    group,
	})
}

// UpdateGroup 更新组
func (h *WazuhHandler) UpdateGroup(c *gin.Context) {
	groupName := c.Param("name")
	
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateGroup(ctx, groupName, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update group: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Group updated successfully",
	})
}

// DeleteGroup 删除组
func (h *WazuhHandler) DeleteGroup(c *gin.Context) {
	groupName := c.Param("name")
	
	ctx := context.Background()
	if err := h.wazuhService.DeleteGroup(ctx, groupName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete group: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Group deleted successfully",
	})
}

// GetGroupAgents 获取组内Agent
// @Summary 获取组内代理列表
// @Description 获取指定组内所有代理的列表
// @Tags Wazuh Groups
// @Accept json
// @Produce json
// @Param name path string true "组名称"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /wazuh/groups/{name}/agents [get]
func (h *WazuhHandler) GetGroupAgents(c *gin.Context) {
	groupName := c.Param("name")
	
	ctx := context.Background()
	agents, err := h.wazuhService.GetGroupAgents(ctx, groupName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get group agents: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    agents,
	})
}

// AddAgentToGroup 添加Agent到组
func (h *WazuhHandler) AddAgentToGroup(c *gin.Context) {
	groupName := c.Param("name")
	
	var req struct {
		AgentID string `json:"agent_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.AddAgentToGroup(ctx, groupName, req.AgentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to add agent to group: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent added to group successfully",
	})
}

// RemoveAgentFromGroup 从组中移除Agent
func (h *WazuhHandler) RemoveAgentFromGroup(c *gin.Context) {
	groupName := c.Param("name")
	agentID := c.Param("agent_id")
	
	ctx := context.Background()
	if err := h.wazuhService.RemoveAgentFromGroup(ctx, groupName, agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to remove agent from group: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Agent removed from group successfully",
	})
}

// GetGroupConfiguration 获取组配置
func (h *WazuhHandler) GetGroupConfiguration(c *gin.Context) {
	groupName := c.Param("name")
	
	ctx := context.Background()
	config, err := h.wazuhService.GetGroupConfiguration(ctx, groupName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get group configuration: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    config,
	})
}

// UpdateGroupConfiguration 更新组配置
func (h *WazuhHandler) UpdateGroupConfiguration(c *gin.Context) {
	groupName := c.Param("name")
	
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateGroupConfiguration(ctx, groupName, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update group configuration: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Group configuration updated successfully",
	})
}

// GetRules 获取规则列表
func (h *WazuhHandler) GetRules(c *gin.Context) {
	ctx := context.Background()
	
	offset := c.DefaultQuery("offset", "0")
	limit := c.DefaultQuery("limit", "100")
	sort := c.Query("sort")
	search := c.Query("search")

	rules, err := h.wazuhService.GetRules(ctx, offset, limit, sort, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get rules: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rules,
	})
}

// GetRule 获取单个规则
func (h *WazuhHandler) GetRule(c *gin.Context) {
	ruleID := c.Param("id")
	
	ctx := context.Background()
	rule, err := h.wazuhService.GetRule(ctx, ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Rule not found: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rule,
	})
}

// CreateRule 创建规则
func (h *WazuhHandler) CreateRule(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	rule, err := h.wazuhService.CreateRule(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create rule: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    rule,
	})
}

// UpdateRule 更新规则
func (h *WazuhHandler) UpdateRule(c *gin.Context) {
	ruleID := c.Param("id")
	
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateRule(ctx, ruleID, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update rule: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Rule updated successfully",
	})
}

// DeleteRule 删除规则
func (h *WazuhHandler) DeleteRule(c *gin.Context) {
	ruleID := c.Param("id")
	
	ctx := context.Background()
	if err := h.wazuhService.DeleteRule(ctx, ruleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete rule: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Rule deleted successfully",
	})
}

// GetRuleFiles 获取规则文件列表
func (h *WazuhHandler) GetRuleFiles(c *gin.Context) {
	ctx := context.Background()
	files, err := h.wazuhService.GetRuleFiles(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get rule files: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    files,
	})
}

// GetRuleFile 获取规则文件内容
func (h *WazuhHandler) GetRuleFile(c *gin.Context) {
	filename := c.Param("filename")
	
	ctx := context.Background()
	content, err := h.wazuhService.GetRuleFile(ctx, filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Rule file not found: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"content": content},
	})
}

// UpdateRuleFile 更新规则文件
func (h *WazuhHandler) UpdateRuleFile(c *gin.Context) {
	filename := c.Param("filename")
	
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateRuleFile(ctx, filename, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update rule file: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Rule file updated successfully",
	})
}

// GetDecoders 获取解码器列表
func (h *WazuhHandler) GetDecoders(c *gin.Context) {
	ctx := context.Background()
	
	offset := c.DefaultQuery("offset", "0")
	limit := c.DefaultQuery("limit", "100")
	sort := c.Query("sort")
	search := c.Query("search")

	decoders, err := h.wazuhService.GetDecoders(ctx, offset, limit, sort, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get decoders: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    decoders,
	})
}

// GetDecoder 获取单个解码器
func (h *WazuhHandler) GetDecoder(c *gin.Context) {
	decoderName := c.Param("name")
	
	ctx := context.Background()
	decoder, err := h.wazuhService.GetDecoder(ctx, decoderName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Decoder not found: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    decoder,
	})
}

// CreateDecoder 创建解码器
func (h *WazuhHandler) CreateDecoder(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	decoder, err := h.wazuhService.CreateDecoder(ctx, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create decoder: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    decoder,
	})
}

// UpdateDecoder 更新解码器
func (h *WazuhHandler) UpdateDecoder(c *gin.Context) {
	decoderName := c.Param("name")
	
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateDecoder(ctx, decoderName, req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update decoder: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Decoder updated successfully",
	})
}

// DeleteDecoder 删除解码器
func (h *WazuhHandler) DeleteDecoder(c *gin.Context) {
	decoderName := c.Param("name")
	
	ctx := context.Background()
	if err := h.wazuhService.DeleteDecoder(ctx, decoderName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete decoder: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Decoder deleted successfully",
	})
}

// GetDecoderFiles 获取解码器文件列表
func (h *WazuhHandler) GetDecoderFiles(c *gin.Context) {
	ctx := context.Background()
	files, err := h.wazuhService.GetDecoderFiles(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get decoder files: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    files,
	})
}

// GetDecoderFile 获取解码器文件内容
func (h *WazuhHandler) GetDecoderFile(c *gin.Context) {
	filename := c.Param("filename")
	
	ctx := context.Background()
	content, err := h.wazuhService.GetDecoderFile(ctx, filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Decoder file not found: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"content": content},
	})
}

// UpdateDecoderFile 更新解码器文件
func (h *WazuhHandler) UpdateDecoderFile(c *gin.Context) {
	filename := c.Param("filename")
	
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateDecoderFile(ctx, filename, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update decoder file: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Decoder file updated successfully",
	})
}

// GetLists 获取CDB列表
func (h *WazuhHandler) GetLists(c *gin.Context) {
	ctx := context.Background()
	
	offset := c.DefaultQuery("offset", "0")
	limit := c.DefaultQuery("limit", "100")
	sort := c.Query("sort")
	search := c.Query("search")

	lists, err := h.wazuhService.GetLists(ctx, offset, limit, sort, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get lists: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    lists,
	})
}

// GetList 获取单个CDB列表
func (h *WazuhHandler) GetList(c *gin.Context) {
	filename := c.Param("filename")
	
	ctx := context.Background()
	list, err := h.wazuhService.GetList(ctx, filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "List not found: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    list,
	})
}

// CreateList 创建CDB列表
func (h *WazuhHandler) CreateList(c *gin.Context) {
	var req struct {
		Filename string `json:"filename" binding:"required"`
		Content  string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.CreateList(ctx, req.Filename, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create list: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "List created successfully",
	})
}

// UpdateList 更新CDB列表
func (h *WazuhHandler) UpdateList(c *gin.Context) {
	filename := c.Param("filename")
	
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.UpdateList(ctx, filename, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update list: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "List updated successfully",
	})
}

// DeleteList 删除CDB列表
func (h *WazuhHandler) DeleteList(c *gin.Context) {
	filename := c.Param("filename")
	
	ctx := context.Background()
	if err := h.wazuhService.DeleteList(ctx, filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete list: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "List deleted successfully",
	})
}

// GetIndexTemplates 获取索引模板
func (h *WazuhHandler) GetIndexTemplates(c *gin.Context) {
	pattern := c.Query("pattern")
	
	ctx := context.Background()
	templates, err := h.wazuhService.GetIndexTemplates(ctx, pattern)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get index templates: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    templates,
	})
}

// CreateIndexTemplate 创建索引模板
func (h *WazuhHandler) CreateIndexTemplate(c *gin.Context) {
	var req struct {
		Name     string                 `json:"name" binding:"required"`
		Template map[string]interface{} `json:"template" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	ctx := context.Background()
	if err := h.wazuhService.CreateIndexTemplate(ctx, req.Name, req.Template); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create index template: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Index template created successfully",
	})
}
