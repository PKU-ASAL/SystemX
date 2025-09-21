package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sysarmor/sysarmor/apps/manager/config"
	"github.com/sysarmor/sysarmor/apps/manager/models"
	"github.com/sysarmor/sysarmor/apps/manager/services/template"
	"github.com/sysarmor/sysarmor/apps/manager/storage"
)

// ResourcesHandler 处理资源下载相关的 HTTP 请求
type ResourcesHandler struct {
	config          *config.Config
	templateService *template.TemplateService
	repo            *storage.Repository
}

// NewResourcesHandler 创建新的 ResourcesHandler
func NewResourcesHandler(db *sql.DB) *ResourcesHandler {
	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{} // 使用默认配置
	}

	// 创建模板服务并加载模板
	templateService := template.NewTemplateService(cfg)
	if err := templateService.LoadTemplates("./shared/templates"); err != nil {
		// 日志记录但不阻止创建
	}

	return &ResourcesHandler{
		config:          cfg,
		templateService: templateService,
		repo:            storage.NewRepository(db),
	}
}

// GetScript 获取部署脚本
// @Summary 获取部署脚本
// @Description 根据部署类型和脚本名称生成个性化的部署脚本
// @Tags resources
// @Accept json
// @Produce text/plain
// @Param deployment_type path string true "部署类型" Enums(agentless,sysarmor-stack,wazuh-hybrid)
// @Param script_name path string true "脚本名称" Enums(setup-terminal.sh,uninstall-terminal.sh,install.sh)
// @Param collector_id query string true "Collector ID"
// @Success 200 {string} string "脚本内容"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "Collector 不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /resources/scripts/{deployment_type}/{script_name} [get]
func (h *ResourcesHandler) GetScript(c *gin.Context) {
	deploymentType := c.Param("deployment_type")
	scriptName := c.Param("script_name")
	collectorID := c.Query("collector_id")

	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id parameter is required",
		})
		return
	}

	// 验证部署类型
	if !isValidResourceDeploymentType(deploymentType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid deployment type: " + deploymentType,
		})
		return
	}

	ctx := c.Request.Context()
	collector, err := h.repo.GetByID(ctx, collectorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Collector not found",
		})
		return
	}

	// 生成脚本内容
	script, err := h.generateScript(collector, deploymentType, scriptName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate script: " + err.Error(),
		})
		return
	}

	// 设置响应头
	filename := h.getScriptFilename(deploymentType, scriptName, collectorID)
	c.Header("Content-Type", "application/x-sh")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Length", strconv.Itoa(len(script)))

	c.String(http.StatusOK, script)
}

// GetBinary 获取二进制文件
// @Summary 获取二进制文件
// @Description 下载存储的二进制文件
// @Tags resources
// @Accept json
// @Produce application/octet-stream
// @Param filename path string true "文件名"
// @Success 200 {file} binary "文件内容"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "文件不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /resources/binaries/{filename} [get]
func (h *ResourcesHandler) GetBinary(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "filename parameter is required",
		})
		return
	}

	// 安全性检查：防止目录遍历攻击
	if !h.isSafeFilename(filename) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid filename",
		})
		return
	}

	// 构建文件路径
	downloadDir := h.config.GetDownloadDir()
	filePath := filepath.Join(downloadDir, filename)

	// 确保文件路径安全
	absDownloadDir, err := filepath.Abs(downloadDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Internal server error",
		})
		return
	}

	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Internal server error",
		})
		return
	}

	// 检查路径是否在允许范围内
	if !strings.HasPrefix(absFilePath, absDownloadDir+string(filepath.Separator)) &&
		absFilePath != absDownloadDir {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid file path",
		})
		return
	}

	// 检查文件是否存在
	if _, err := os.Stat(absFilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "File not found",
		})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Internal server error",
		})
		return
	}

	// 设置响应头
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Type", "application/octet-stream")

	// 发送文件
	c.File(absFilePath)
}

// GetConfig 获取配置文件
// @Summary 获取配置文件
// @Description 根据部署类型和配置名称生成个性化的配置文件
// @Tags resources
// @Accept json
// @Produce text/plain
// @Param deployment_type path string true "部署类型" Enums(agentless,sysarmor-stack,wazuh-hybrid)
// @Param config_name path string true "配置名称" Enums(cfg.yaml,ossec.conf,audit-rules)
// @Param collector_id query string true "Collector ID"
// @Success 200 {string} string "配置内容"
// @Failure 400 {object} map[string]interface{} "请求参数错误"
// @Failure 404 {object} map[string]interface{} "Collector 不存在"
// @Failure 500 {object} map[string]interface{} "服务器内部错误"
// @Router /resources/configs/{deployment_type}/{config_name} [get]
func (h *ResourcesHandler) GetConfig(c *gin.Context) {
	deploymentType := c.Param("deployment_type")
	configName := c.Param("config_name")
	collectorID := c.Query("collector_id")

	if collectorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "collector_id parameter is required",
		})
		return
	}

	// 验证部署类型
	if !isValidResourceDeploymentType(deploymentType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid deployment type: " + deploymentType,
		})
		return
	}

	ctx := c.Request.Context()
	collector, err := h.repo.GetByID(ctx, collectorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Collector not found",
		})
		return
	}

	// 生成配置内容
	config, err := h.generateConfig(collector, deploymentType, configName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate config: " + err.Error(),
		})
		return
	}

	// 设置响应头
	filename := h.getConfigFilename(deploymentType, configName, collectorID)
	c.Header("Content-Type", "text/plain")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Header("Content-Length", strconv.Itoa(len(config)))

	c.String(http.StatusOK, config)
}

// 辅助方法

// generateScript 生成脚本内容
func (h *ResourcesHandler) generateScript(collector *models.Collector, deploymentType, scriptName string) (string, error) {
	// 创建模板数据
	templateData, err := h.templateService.NewTemplateData(collector)
	if err != nil {
		return "", err
	}

	// 设置 Manager URL
	templateData.ManagerURL = h.config.GetManagerURL()

	// 根据部署类型和脚本名称选择模板
	var templateName string
	switch deploymentType {
	case models.DeploymentTypeAgentless:
		if strings.Contains(scriptName, "uninstall") {
			templateName = "agentless/uninstall-terminal.sh"
		} else {
			templateName = "agentless/setup-terminal.sh"
		}
	case models.DeploymentTypeSysArmor:
		// OpenTelemetry Collector 特殊处理
		return h.generateOtelCollectorScript(templateData)
	case models.DeploymentTypeWazuh:
		if strings.Contains(scriptName, "uninstall") {
			templateName = "wazuh-hybrid/uninstall-wazuh.sh"
		} else {
			templateName = "wazuh-hybrid/install-wazuh.sh"
		}
	default:
		return "", fmt.Errorf("unsupported deployment type: %s", deploymentType)
	}

	// 渲染模板
	script, err := h.templateService.RenderTemplate(templateName, templateData)
	if err != nil {
		return "", err
	}

	return script, nil
}

// generateOtelCollectorScript 生成 OpenTelemetry Collector 脚本
func (h *ResourcesHandler) generateOtelCollectorScript(templateData *template.TemplateData) (string, error) {
	// 先渲染配置文件模板
	configContent, err := h.templateService.RenderTemplate("collector/cfg.yaml", templateData)
	if err != nil {
		return "", fmt.Errorf("failed to render config template: %w", err)
	}

	// 设置配置文件内容到模板数据中
	templateData.ExtraCfgData = configContent

	// 再渲染安装脚本模板
	script, err := h.templateService.RenderTemplate("collector/install.sh", templateData)
	if err != nil {
		return "", fmt.Errorf("failed to render install script template: %w", err)
	}

	return script, nil
}

// generateConfig 生成配置内容
func (h *ResourcesHandler) generateConfig(collector *models.Collector, deploymentType, configName string) (string, error) {
	// 验证部署类型匹配
	if collector.DeploymentType != deploymentType {
		return "", fmt.Errorf("deployment type mismatch: collector is %s but requested %s", collector.DeploymentType, deploymentType)
	}

	// 创建模板数据
	templateData, err := h.templateService.NewTemplateData(collector)
	if err != nil {
		return "", err
	}

	// 根据部署类型和配置名称选择模板
	var templateName string
	switch deploymentType {
	case models.DeploymentTypeSysArmor:
		if configName == "cfg.yaml" {
			templateName = "collector/cfg.yaml"
		}
	case models.DeploymentTypeAgentless:
		if configName == "audit-rules" {
			templateName = "agentless/audit-rules"
		}
		// 注意：agentless模式不支持cfg.yaml，因为它不需要安装collector程序
	case models.DeploymentTypeWazuh:
		if configName == "ossec.conf" {
			templateName = "wazuh/ossec.conf"
		}
	default:
		return "", fmt.Errorf("unsupported config: %s/%s", deploymentType, configName)
	}

	if templateName == "" {
		return "", fmt.Errorf("no template found for: %s/%s", deploymentType, configName)
	}

	// 渲染模板
	config, err := h.templateService.RenderTemplate(templateName, templateData)
	if err != nil {
		return "", err
	}

	return config, nil
}

// getScriptFilename 生成脚本文件名
func (h *ResourcesHandler) getScriptFilename(deploymentType, scriptName, collectorID string) string {
	prefix := collectorID[:8]
	switch deploymentType {
	case models.DeploymentTypeAgentless:
		if strings.Contains(scriptName, "uninstall") {
			return "uninstall-terminal-" + prefix + ".sh"
		}
		return "setup-terminal-" + prefix + ".sh"
	case models.DeploymentTypeSysArmor:
		if strings.Contains(scriptName, "uninstall") {
			return "uninstall-collector-" + prefix + ".sh"
		}
		return "install-collector-" + prefix + ".sh"
	case models.DeploymentTypeWazuh:
		if strings.Contains(scriptName, "uninstall") {
			return "uninstall-wazuh-" + prefix + ".sh"
		}
		return "install-wazuh-" + prefix + ".sh"
	default:
		return scriptName + "-" + prefix + ".sh"
	}
}

// getConfigFilename 生成配置文件名
func (h *ResourcesHandler) getConfigFilename(deploymentType, configName, collectorID string) string {
	prefix := collectorID[:8]
	switch configName {
	case "cfg.yaml":
		return "otelcol-" + prefix + ".yaml"
	case "audit-rules":
		return "audit-rules-" + prefix + ".rules"
	case "ossec.conf":
		return "ossec-" + prefix + ".conf"
	default:
		return configName + "-" + prefix
	}
}

// isSafeFilename 检查文件名是否安全
func (h *ResourcesHandler) isSafeFilename(filename string) bool {
	cleanFilename := filepath.Clean(filename)

	// 检查是否包含路径遍历字符
	if strings.Contains(cleanFilename, "..") ||
		strings.Contains(cleanFilename, "/") ||
		strings.Contains(cleanFilename, "\\") ||
		strings.HasPrefix(cleanFilename, ".") {
		return false
	}

	// 检查文件名是否为空
	if strings.TrimSpace(cleanFilename) == "" {
		return false
	}

	return true
}

// isValidResourceDeploymentType 验证资源部署类型 (避免与 collector.go 中的函数重名)
func isValidResourceDeploymentType(deploymentType string) bool {
	validTypes := []string{
		models.DeploymentTypeAgentless,
		models.DeploymentTypeSysArmor,
		models.DeploymentTypeWazuh,
		"collector",
	}

	for _, validType := range validTypes {
		if deploymentType == validType {
			return true
		}
	}
	return false
}
