package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sysarmor/sysarmor/services/manager/internal/config"
	"github.com/sysarmor/sysarmor/services/manager/internal/models"
	"github.com/sysarmor/sysarmor/services/manager/internal/services/health"
	kafkaService "github.com/sysarmor/sysarmor/services/manager/internal/services/kafka"
	"github.com/sysarmor/sysarmor/services/manager/internal/services/template"
	"github.com/sysarmor/sysarmor/services/manager/internal/storage"
)

// CollectorHandler 处理 Collector 相关的 HTTP 请求
type CollectorHandler struct {
	repo            *storage.Repository
	config          *config.Config
	healthChecker   *health.HealthChecker
	kafkaService    *kafkaService.KafkaService
	templateService *template.TemplateService
}

// NewCollectorHandler 创建新的 CollectorHandler
func NewCollectorHandler(db *sql.DB) *CollectorHandler {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("⚠️ Warning: Failed to load config: %v, using defaults\n", err)
		// 创建一个默认配置以防止崩溃
		cfg = &config.Config{}
	}
	
	// 创建模板服务并加载模板
	templateService := template.NewTemplateService()
	if err := templateService.LoadTemplates("./templates"); err != nil {
		fmt.Printf("⚠️ Warning: Failed to load templates: %v\n", err)
	}
	
	return &CollectorHandler{
		repo:            storage.NewRepository(db),
		config:          cfg,
		healthChecker:   health.NewHealthChecker(),
		kafkaService:    kafkaService.NewKafkaService(cfg.GetKafkaBrokerList()),
		templateService: templateService,
	}
}

// Register 处理 Collector 注册请求
// @Summary 注册新的 Collector
// @Description 为终端设备注册一个新的 Collector，分配唯一 ID 并选择健康的 Worker
// @Tags collectors
// @Accept json
// @Produce json
// @Param request body models.RegisterRequest true "注册请求参数"
// @Success 200 {object} models.RegisterResponse "注册成功"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 501 {object} map[string]interface{} "部署类型未实现"
// @Failure 503 {object} map[string]interface{} "没有可用的健康 Worker"
// @Router /collectors/register [post]
func (h *CollectorHandler) Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format: " + err.Error(),
		})
		return
	}

	// 生成唯一的 collector ID
	collectorID := uuid.New().String()

	// 选择健康的 Worker
	ctx := c.Request.Context()
	selectedWorker := h.healthChecker.SelectHealthyWorker(ctx)
	if selectedWorker == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "No healthy workers available",
		})
		return
	}

	workerURL := selectedWorker.URL

	// 验证部署类型
	deploymentType := req.DeploymentType
	if deploymentType == "" {
		deploymentType = models.DeploymentTypeAgentless // 默认值
	}

	// 验证部署类型是否支持
	if !isValidDeploymentType(deploymentType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Unsupported deployment_type: %s. Supported types: %s", deploymentType, getSupportedDeploymentTypes()),
		})
		return
	}

	// 目前只支持 agentless 类型
	if deploymentType != models.DeploymentTypeAgentless {
		c.JSON(http.StatusNotImplemented, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Deployment type '%s' is not implemented yet. Currently only 'agentless' is supported.", deploymentType),
		})
		return
	}

	// 创建 collector 记录以生成 topic 名称
	collector := &models.Collector{
		ID:             uuid.New(),
		CollectorID:    collectorID,
		Hostname:       req.Hostname,
		IPAddress:      req.IPAddress,
		OSType:         req.OSType,
		OSVersion:      req.OSVersion,
		Status:         models.CollectorStatusActive,
		WorkerAddress:  workerURL,
		DeploymentType: deploymentType,
		Metadata:       req.Metadata, // 设置元数据
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// 生成基于部署类型的 Kafka topic 名称
	topicName := collector.GetTopicName()
	collector.KafkaTopic = topicName

	// 创建 Kafka topic
	createTopicReq := &kafkaService.CreateTopicRequest{
		Name:              topicName,
		Partitions:        3,
		ReplicationFactor: 1,
	}
	if err := h.kafkaService.CreateTopic(ctx, createTopicReq); err != nil {
		fmt.Printf("⚠️ Warning: Failed to create Kafka topic %s: %v\n", topicName, err)
		// 不阻止注册流程，只记录警告
	}

	// 保存到数据库
	if err := h.repo.Create(ctx, collector); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save collector: " + err.Error(),
		})
		return
	}

	// 构造响应
	var resp models.RegisterResponse
	resp.Success = true
	resp.Data.CollectorID = collectorID
	resp.Data.WorkerURL = workerURL
	resp.Data.ScriptDownloadURL = fmt.Sprintf("/api/v1/scripts/setup-terminal.sh?collector_id=%s", collectorID)

	// 记录日志
	fmt.Printf("✅ Collector registered: %s (hostname: %s, worker: %s, topic: %s)\n",
		collectorID, req.Hostname, workerURL, topicName)

	c.JSON(http.StatusOK, resp)
}

// GetStatus 获取 Collector 状态
// @Summary 获取 Collector 状态
// @Description 根据 Collector ID 获取其详细状态信息
// @Tags collectors
// @Accept json
// @Produce json
// @Param id path string true "Collector ID"
// @Success 200 {object} map[string]interface{} "Collector 状态信息"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "Collector 不存在"
// @Router /collectors/{id} [get]
func (h *CollectorHandler) GetStatus(c *gin.Context) {
	collectorID := c.Param("id")
	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id is required",
		})
		return
	}

	ctx := c.Request.Context()
	collector, err := h.repo.GetByID(ctx, collectorID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Collector not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Database error: " + err.Error(),
			})
		}
		return
	}

	// 构造状态响应
	status := models.CollectorStatus{
		CollectorID:   collector.CollectorID,
		Status:        collector.Status,
		Hostname:      collector.Hostname,
		IPAddress:     collector.IPAddress,
		WorkerAddress: collector.WorkerAddress,
		KafkaTopic:    collector.KafkaTopic,
		Metadata:      collector.Metadata, // 包含元数据
		LastHeartbeat: collector.LastHeartbeat,
		CreatedAt:     collector.CreatedAt,
		UpdatedAt:     collector.UpdatedAt,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
		"scripts": gin.H{
			"install_script_url":   fmt.Sprintf("/api/v1/scripts/setup-terminal.sh?collector_id=%s", collector.CollectorID),
			"uninstall_script_url": fmt.Sprintf("/api/v1/scripts/uninstall-terminal.sh?collector_id=%s", collector.CollectorID),
		},
	})
}

// DownloadScript 生成并下载终端配置脚本
func (h *CollectorHandler) DownloadScript(c *gin.Context) {
	collectorID := c.Query("collector_id")
	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id parameter is required",
		})
		return
	}

	ctx := c.Request.Context()
	collector, err := h.repo.GetByID(ctx, collectorID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Collector not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Database error: " + err.Error(),
			})
		}
		return
	}

	// 使用模板生成脚本内容
	script, err := h.generateScriptFromTemplate(collector)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate script: " + err.Error(),
		})
		return
	}

	// 设置响应头
	filename := fmt.Sprintf("setup-terminal-%s.sh", collector.CollectorID[:8])
	c.Header("Content-Type", "application/x-sh")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Length", strconv.Itoa(len(script)))

	// 记录日志
	fmt.Printf("📥 Script downloaded for collector: %s (filename: %s)\n", collectorID, filename)

	c.String(http.StatusOK, script)
}

// ListCollectors 列出所有 Collectors，支持 Query Parameters 过滤
// @Summary 列出所有 Collectors
// @Description 获取所有 Collector 列表，支持分页、过滤和排序
// @Tags collectors
// @Accept json
// @Produce json
// @Param page query int false "页码，默认为1"
// @Param limit query int false "每页数量，默认为20，最大100"
// @Param status query string false "按状态过滤"
// @Param group query string false "按分组过滤"
// @Param environment query string false "按环境过滤"
// @Param owner query string false "按负责人过滤"
// @Param tags query string false "按标签过滤，多个标签用逗号分隔"
// @Param sort query string false "排序字段"
// @Param order query string false "排序方向，asc或desc，默认desc"
// @Success 200 {object} map[string]interface{} "Collector 列表"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /collectors [get]
func (h *CollectorHandler) ListCollectors(c *gin.Context) {
	ctx := c.Request.Context()
	
	// 解析查询参数
	filters := h.parseQueryFilters(c)
	pagination := h.parseQueryPagination(c)
	sort := h.parseQuerySort(c)

	// 如果没有任何过滤条件，使用原来的 List 方法
	if filters == nil && pagination == nil && sort == nil {
		collectors, err := h.repo.List(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Database error: " + err.Error(),
			})
			return
		}

		// 转换为状态响应
		statuses := make([]*models.CollectorStatus, len(collectors))
		for i, collector := range collectors {
			statuses[i] = &models.CollectorStatus{
				CollectorID:   collector.CollectorID,
				Status:        collector.Status,
				Hostname:      collector.Hostname,
				IPAddress:     collector.IPAddress,
				WorkerAddress: collector.WorkerAddress,
				KafkaTopic:    collector.KafkaTopic,
				Metadata:      collector.Metadata,
				LastHeartbeat: collector.LastHeartbeat,
				CreatedAt:     collector.CreatedAt,
				UpdatedAt:     collector.UpdatedAt,
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data": gin.H{
				"collectors": statuses,
				"total":      len(statuses),
			},
		})
		return
	}

	// 使用搜索方法
	collectors, total, err := h.repo.SearchCollectors(ctx, filters, pagination, sort)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Database error: " + err.Error(),
		})
		return
	}

	// 转换为状态响应
	statuses := make([]*models.CollectorStatus, len(collectors))
	for i, collector := range collectors {
		statuses[i] = &models.CollectorStatus{
			CollectorID:   collector.CollectorID,
			Status:        collector.Status,
			Hostname:      collector.Hostname,
			IPAddress:     collector.IPAddress,
			WorkerAddress: collector.WorkerAddress,
			KafkaTopic:    collector.KafkaTopic,
			Metadata:      collector.Metadata,
			LastHeartbeat: collector.LastHeartbeat,
			CreatedAt:     collector.CreatedAt,
			UpdatedAt:     collector.UpdatedAt,
		}
	}

	// 构建响应
	response := gin.H{
		"success": true,
		"data": gin.H{
			"collectors": statuses,
			"total":      total,
		},
	}

	// 如果有分页，添加分页信息
	if pagination != nil {
		totalPages := (total + pagination.Limit - 1) / pagination.Limit
		response["data"].(gin.H)["page"] = pagination.Page
		response["data"].(gin.H)["limit"] = pagination.Limit
		response["data"].(gin.H)["total_pages"] = totalPages
	}

	c.JSON(http.StatusOK, response)
}

// Heartbeat 处理心跳请求（预留接口）
func (h *CollectorHandler) Heartbeat(c *gin.Context) {
	collectorID := c.Param("id")
	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id is required",
		})
		return
	}

	// 简单的心跳更新
	ctx := c.Request.Context()
	if err := h.repo.UpdateHeartbeat(ctx, collectorID); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Collector not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to update heartbeat: " + err.Error(),
			})
		}
		return
	}

	// 返回简单响应
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"next_heartbeat_interval": 30,
			"timestamp":               time.Now(),
		},
	})
}

// 辅助函数

// isValidDeploymentType 验证部署类型是否有效
func isValidDeploymentType(deploymentType string) bool {
	validTypes := []string{
		models.DeploymentTypeAgentless,
		models.DeploymentTypeSysArmor,
		models.DeploymentTypeWazuh,
	}
	
	for _, validType := range validTypes {
		if deploymentType == validType {
			return true
		}
	}
	return false
}

// getSupportedDeploymentTypes 获取支持的部署类型列表
func getSupportedDeploymentTypes() string {
	return fmt.Sprintf("[%s, %s, %s]", 
		models.DeploymentTypeAgentless,
		models.DeploymentTypeSysArmor,
		models.DeploymentTypeWazuh,
	)
}


// parseWorkerURL 解析 Worker URL
func parseWorkerURL(workerURL string) (host, port string) {
	// 处理格式: http://localhost:514:http://localhost
	// 我们需要提取 host 和 port
	if strings.HasPrefix(workerURL, "http://") {
		// 去掉 http:// 前缀
		urlWithoutProtocol := strings.TrimPrefix(workerURL, "http://")
		// 分割获取第一部分 (localhost:514)
		parts := strings.Split(urlWithoutProtocol, ":")
		if len(parts) >= 2 {
			host = parts[0]  // localhost
			port = parts[1]  // 514
			return host, port
		}
	}
	
	// 回退到简单的 host:port 格式
	parts := strings.Split(workerURL, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "localhost", "514" // 默认值
}

// generateScriptFromTemplate 使用模板生成脚本
func (h *CollectorHandler) generateScriptFromTemplate(collector *models.Collector) (string, error) {
	// 创建模板数据
	templateData, err := template.NewTemplateData(collector)
	if err != nil {
		return "", fmt.Errorf("failed to create template data: %w", err)
	}

	// 根据部署类型选择模板
	var templateName string
	switch collector.DeploymentType {
	case models.DeploymentTypeAgentless:
		templateName = "agentless/setup-terminal.sh"
	case models.DeploymentTypeSysArmor:
		templateName = "sysarmor-stack/install-collector.sh"
	case models.DeploymentTypeWazuh:
		templateName = "wazuh-hybrid/install-wazuh.sh"
	default:
		return "", fmt.Errorf("unsupported deployment type: %s", collector.DeploymentType)
	}

	// 渲染模板
	script, err := h.templateService.RenderTemplate(templateName, templateData)
	if err != nil {
		return "", fmt.Errorf("failed to render template %s: %w", templateName, err)
	}

	return script, nil
}

// generateUninstallScriptFromTemplate 使用模板生成卸载脚本
func (h *CollectorHandler) generateUninstallScriptFromTemplate(collector *models.Collector) (string, error) {
	// 创建模板数据
	templateData, err := template.NewTemplateData(collector)
	if err != nil {
		return "", fmt.Errorf("failed to create template data: %w", err)
	}

	// 根据部署类型选择卸载模板
	var templateName string
	switch collector.DeploymentType {
	case models.DeploymentTypeAgentless:
		templateName = "agentless/uninstall-terminal.sh"
	case models.DeploymentTypeSysArmor:
		templateName = "sysarmor-stack/uninstall-collector.sh"
	case models.DeploymentTypeWazuh:
		templateName = "wazuh-hybrid/uninstall-wazuh.sh"
	default:
		return "", fmt.Errorf("unsupported deployment type: %s", collector.DeploymentType)
	}

	// 渲染模板
	script, err := h.templateService.RenderTemplate(templateName, templateData)
	if err != nil {
		return "", fmt.Errorf("failed to render uninstall template %s: %w", templateName, err)
	}

	return script, nil
}

// UpdateMetadata 更新 Collector 元数据
func (h *CollectorHandler) UpdateMetadata(c *gin.Context) {
	collectorID := c.Param("id")
	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id is required",
		})
		return
	}

	var req models.UpdateMetadataRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format: " + err.Error(),
		})
		return
	}

	ctx := c.Request.Context()
	if err := h.repo.UpdateMetadata(ctx, collectorID, req.Metadata); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Collector not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to update metadata: " + err.Error(),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Metadata updated successfully",
	})
}

// Delete 删除 Collector
func (h *CollectorHandler) Delete(c *gin.Context) {
	collectorID := c.Param("id")
	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id is required",
		})
		return
	}

	ctx := c.Request.Context()
	
	// 首先获取 collector 信息，用于清理相关资源
	collector, err := h.repo.GetByID(ctx, collectorID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Collector not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Database error: " + err.Error(),
			})
		}
		return
	}

	// 检查是否强制删除
	force := c.Query("force") == "true"
	
	// 如果不是强制删除，先将状态设置为 inactive
	if !force {
		if err := h.repo.UpdateStatus(ctx, collectorID, models.CollectorStatusInactive); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to deactivate collector: " + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Collector deactivated successfully. Use force=true to permanently delete.",
			"data": gin.H{
				"collector_id": collectorID,
				"status":       models.CollectorStatusInactive,
				"uninstall_script_url": fmt.Sprintf("/api/v1/scripts/uninstall-terminal.sh?collector_id=%s", collectorID),
			},
		})
		return
	}

	// 强制删除：清理相关资源
	// 1. 尝试删除 Kafka topic（可选，因为可能有其他数据）
	if collector.KafkaTopic != "" {
            if err := h.kafkaService.DeleteTopic(ctx, collector.KafkaTopic, false); err != nil {
			fmt.Printf("⚠️ Warning: Failed to delete Kafka topic %s: %v\n", collector.KafkaTopic, err)
			// 不阻止删除流程，只记录警告
		}
	}

	// 2. 从数据库中删除记录
	if err := h.repo.Delete(ctx, collectorID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete collector: " + err.Error(),
		})
		return
	}

	// 记录日志
	fmt.Printf("🗑️ Collector deleted: %s (hostname: %s, topic: %s)\n",
		collectorID, collector.Hostname, collector.KafkaTopic)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Collector deleted successfully",
		"data": gin.H{
			"collector_id": collectorID,
			"hostname":     collector.Hostname,
			"kafka_topic":  collector.KafkaTopic,
		},
	})
}

// Unregister 注销 Collector（软删除）
func (h *CollectorHandler) Unregister(c *gin.Context) {
	collectorID := c.Param("id")
	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id is required",
		})
		return
	}

	ctx := c.Request.Context()
	
	// 获取 collector 信息
	collector, err := h.repo.GetByID(ctx, collectorID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Collector not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Database error: " + err.Error(),
			})
		}
		return
	}

	// 将状态设置为 unregistered
	if err := h.repo.UpdateStatus(ctx, collectorID, models.CollectorStatusUnregistered); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to unregister collector: " + err.Error(),
		})
		return
	}

	// 记录日志
	fmt.Printf("📤 Collector unregistered: %s (hostname: %s)\n", collectorID, collector.Hostname)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Collector unregistered successfully",
		"data": gin.H{
			"collector_id":         collectorID,
			"status":               models.CollectorStatusUnregistered,
			"uninstall_script_url": fmt.Sprintf("/api/v1/scripts/uninstall-terminal.sh?collector_id=%s", collectorID),
		},
	})
}

// DownloadUninstallScript 生成并下载卸载脚本
func (h *CollectorHandler) DownloadUninstallScript(c *gin.Context) {
	collectorID := c.Query("collector_id")
	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id parameter is required",
		})
		return
	}

	ctx := c.Request.Context()
	collector, err := h.repo.GetByID(ctx, collectorID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   "Collector not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Database error: " + err.Error(),
			})
		}
		return
	}

	// 使用模板生成卸载脚本内容
	script, err := h.generateUninstallScriptFromTemplate(collector)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate uninstall script: " + err.Error(),
		})
		return
	}

	// 设置响应头
	filename := fmt.Sprintf("uninstall-terminal-%s.sh", collector.CollectorID[:8])
	c.Header("Content-Type", "application/x-sh")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Length", strconv.Itoa(len(script)))

	// 记录日志
	fmt.Printf("📥 Uninstall script downloaded for collector: %s (filename: %s)\n", collectorID, filename)

	c.String(http.StatusOK, script)
}

// GetByGroup 根据分组获取 Collectors
func (h *CollectorHandler) GetByGroup(c *gin.Context) {
	group := c.Param("group")
	if group == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "group is required",
		})
		return
	}

	ctx := c.Request.Context()
	collectors, err := h.repo.GetByGroup(ctx, group)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Database error: " + err.Error(),
		})
		return
	}

	// 转换为状态响应
	statuses := make([]*models.CollectorStatus, len(collectors))
	for i, collector := range collectors {
		statuses[i] = &models.CollectorStatus{
			CollectorID:   collector.CollectorID,
			Status:        collector.Status,
			Hostname:      collector.Hostname,
			IPAddress:     collector.IPAddress,
			WorkerAddress: collector.WorkerAddress,
			KafkaTopic:    collector.KafkaTopic,
			Metadata:      collector.Metadata,
			LastHeartbeat: collector.LastHeartbeat,
			CreatedAt:     collector.CreatedAt,
			UpdatedAt:     collector.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"group":       group,
			"collectors":  statuses,
			"total":       len(statuses),
		},
	})
}

// GetByTag 根据标签获取 Collectors
func (h *CollectorHandler) GetByTag(c *gin.Context) {
	tag := c.Param("tag")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "tag is required",
		})
		return
	}

	ctx := c.Request.Context()
	collectors, err := h.repo.GetByTag(ctx, tag)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Database error: " + err.Error(),
		})
		return
	}

	// 转换为状态响应
	statuses := make([]*models.CollectorStatus, len(collectors))
	for i, collector := range collectors {
		statuses[i] = &models.CollectorStatus{
			CollectorID:   collector.CollectorID,
			Status:        collector.Status,
			Hostname:      collector.Hostname,
			IPAddress:     collector.IPAddress,
			WorkerAddress: collector.WorkerAddress,
			KafkaTopic:    collector.KafkaTopic,
			Metadata:      collector.Metadata,
			LastHeartbeat: collector.LastHeartbeat,
			CreatedAt:     collector.CreatedAt,
			UpdatedAt:     collector.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"tag":         tag,
			"collectors":  statuses,
			"total":       len(statuses),
		},
	})
}

// GetByEnvironment 根据环境获取 Collectors
func (h *CollectorHandler) GetByEnvironment(c *gin.Context) {
	environment := c.Param("environment")
	if environment == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "environment is required",
		})
		return
	}

	ctx := c.Request.Context()
	collectors, err := h.repo.GetByEnvironment(ctx, environment)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Database error: " + err.Error(),
		})
		return
	}

	// 转换为状态响应
	statuses := make([]*models.CollectorStatus, len(collectors))
	for i, collector := range collectors {
		statuses[i] = &models.CollectorStatus{
			CollectorID:   collector.CollectorID,
			Status:        collector.Status,
			Hostname:      collector.Hostname,
			IPAddress:     collector.IPAddress,
			WorkerAddress: collector.WorkerAddress,
			KafkaTopic:    collector.KafkaTopic,
			Metadata:      collector.Metadata,
			LastHeartbeat: collector.LastHeartbeat,
			CreatedAt:     collector.CreatedAt,
			UpdatedAt:     collector.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"environment": environment,
			"collectors":  statuses,
			"total":       len(statuses),
		},
	})
}

// GetByOwner 根据负责人获取 Collectors
func (h *CollectorHandler) GetByOwner(c *gin.Context) {
	owner := c.Param("owner")
	if owner == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "owner is required",
		})
		return
	}

	ctx := c.Request.Context()
	collectors, err := h.repo.GetByOwner(ctx, owner)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Database error: " + err.Error(),
		})
		return
	}

	// 转换为状态响应
	statuses := make([]*models.CollectorStatus, len(collectors))
	for i, collector := range collectors {
		statuses[i] = &models.CollectorStatus{
			CollectorID:   collector.CollectorID,
			Status:        collector.Status,
			Hostname:      collector.Hostname,
			IPAddress:     collector.IPAddress,
			WorkerAddress: collector.WorkerAddress,
			KafkaTopic:    collector.KafkaTopic,
			Metadata:      collector.Metadata,
			LastHeartbeat: collector.LastHeartbeat,
			CreatedAt:     collector.CreatedAt,
			UpdatedAt:     collector.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"owner":      owner,
			"collectors": statuses,
			"total":      len(statuses),
		},
	})
}

// parseQueryFilters 解析查询参数中的过滤条件
func (h *CollectorHandler) parseQueryFilters(c *gin.Context) *models.CollectorFilters {
	filters := &models.CollectorFilters{}
	hasFilters := false

	// 解析标签（支持逗号分隔）
	if tagsParam := c.Query("tags"); tagsParam != "" {
		filters.Tags = strings.Split(tagsParam, ",")
		// 清理空白字符
		for i, tag := range filters.Tags {
			filters.Tags[i] = strings.TrimSpace(tag)
		}
		hasFilters = true
	}

	// 单个标签（向后兼容）
	if tag := c.Query("tag"); tag != "" {
		filters.Tags = append(filters.Tags, tag)
		hasFilters = true
	}

	// 其他过滤条件
	if group := c.Query("group"); group != "" {
		filters.Group = group
		hasFilters = true
	}

	if environment := c.Query("environment"); environment != "" {
		filters.Environment = environment
		hasFilters = true
	}

	if owner := c.Query("owner"); owner != "" {
		filters.Owner = owner
		hasFilters = true
	}

	if status := c.Query("status"); status != "" {
		filters.Status = status
		hasFilters = true
	}

	if region := c.Query("region"); region != "" {
		filters.Region = region
		hasFilters = true
	}

	if purpose := c.Query("purpose"); purpose != "" {
		filters.Purpose = purpose
		hasFilters = true
	}

	if !hasFilters {
		return nil
	}

	return filters
}

// parseQueryPagination 解析查询参数中的分页信息
func (h *CollectorHandler) parseQueryPagination(c *gin.Context) *models.PaginationRequest {
	pageStr := c.Query("page")
	limitStr := c.Query("limit")

	if pageStr == "" && limitStr == "" {
		return nil
	}

	pagination := &models.PaginationRequest{
		Page:  1,  // 默认第一页
		Limit: 20, // 默认每页20条
	}

	if pageStr != "" {
		if page, err := strconv.Atoi(pageStr); err == nil && page > 0 {
			pagination.Page = page
		}
	}

	if limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 100 {
			pagination.Limit = limit
		}
	}

	return pagination
}

// parseQuerySort 解析查询参数中的排序信息
func (h *CollectorHandler) parseQuerySort(c *gin.Context) *models.SortRequest {
	field := c.Query("sort")
	order := c.Query("order")

	if field == "" {
		return nil
	}

	sort := &models.SortRequest{
		Field: field,
		Order: "desc", // 默认降序
	}

	if order == "asc" || order == "desc" {
		sort.Order = order
	}

	return sort
}
