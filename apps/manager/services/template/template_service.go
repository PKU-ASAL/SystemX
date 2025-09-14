package template

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/sysarmor/sysarmor/apps/manager/config"
	"github.com/sysarmor/sysarmor/apps/manager/models"
)

// TemplateService 模板服务
type TemplateService struct {
	templates map[string]*template.Template
	config    *config.Config
}

// NewTemplateService 创建新的模板服务
func NewTemplateService(cfg *config.Config) *TemplateService {
	return &TemplateService{
		templates: make(map[string]*template.Template),
		config:    cfg,
	}
}

// LoadTemplates 加载所有模板文件
func (ts *TemplateService) LoadTemplates(templateDir string) error {
	return filepath.WalkDir(templateDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 只处理 .tmpl 文件
		if d.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}

		// 生成模板名称 (相对路径，去掉 .tmpl 后缀)
		relPath, err := filepath.Rel(templateDir, path)
		if err != nil {
			return err
		}
		templateName := strings.TrimSuffix(relPath, ".tmpl")

		// 解析模板
		tmpl, err := template.ParseFiles(path)
		if err != nil {
			return fmt.Errorf("failed to parse template %s: %w", path, err)
		}

		ts.templates[templateName] = tmpl
		fmt.Printf("✅ Loaded template: %s\n", templateName)
		return nil
	})
}

// TemplateData 模板数据结构
type TemplateData struct {
	// Collector 基本信息
	CollectorID    string
	Hostname       string
	IPAddress      string
	OSType         string
	OSVersion      string
	DeploymentType string
	WorkerAddress  string
	KafkaTopic     string

	// 解析后的 Worker 信息
	WorkerHost string
	WorkerPort string

	// Manager 信息
	ManagerURL string

	// 生成时间
	GeneratedAt string

	// Agentless 特定数据
	AuditRules string
	
	// OpenTelemetry Collector 特定数据
	ExtraCfgData string
}

// NewTemplateData 从 Collector 创建模板数据
func (ts *TemplateService) NewTemplateData(collector *models.Collector) (*TemplateData, error) {
	// 解析 Worker URL
	workerHost, workerPort := ts.parseWorkerURL(collector.WorkerAddress)

	data := &TemplateData{
		CollectorID:    collector.CollectorID,
		Hostname:       collector.Hostname,
		IPAddress:      collector.IPAddress,
		OSType:         collector.OSType,
		OSVersion:      collector.OSVersion,
		DeploymentType: collector.DeploymentType,
		WorkerAddress:  collector.WorkerAddress,
		KafkaTopic:     collector.KafkaTopic,
		WorkerHost:     workerHost,
		WorkerPort:     workerPort,
		GeneratedAt:    time.Now().Format("2006-01-02 15:04:05 MST"),
	}

	return data, nil
}

// RenderTemplate 渲染指定模板
func (ts *TemplateService) RenderTemplate(templateName string, data *TemplateData) (string, error) {
	tmpl, exists := ts.templates[templateName]
	if !exists {
		return "", fmt.Errorf("template not found: %s", templateName)
	}

	// 如果是 agentless 脚本，需要先渲染 audit rules
	if templateName == "agentless/setup-terminal.sh" {
		auditRules, err := ts.RenderTemplate("agentless/audit-rules", data)
		if err != nil {
			return "", fmt.Errorf("failed to render audit rules: %w", err)
		}
		data.AuditRules = auditRules
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// GetAvailableTemplates 获取可用的模板列表
func (ts *TemplateService) GetAvailableTemplates() []string {
	templates := make([]string, 0, len(ts.templates))
	for name := range ts.templates {
		templates = append(templates, name)
	}
	return templates
}

// parseWorkerURL 解析 Worker URL
func (ts *TemplateService) parseWorkerURL(workerURL string) (host, port string) {
	// 处理格式: http://localhost:514:http://localhost
	// 我们需要提取 host 和 port
	if strings.HasPrefix(workerURL, "http://") {
		// 去掉 http:// 前缀
		urlWithoutProtocol := strings.TrimPrefix(workerURL, "http://")
		// 分割获取第一部分 (localhost:514)
		parts := strings.Split(urlWithoutProtocol, ":")
		if len(parts) >= 2 {
			host = parts[0] // localhost
			port = parts[1] // 端口
			return host, port
		}
	}

	// 回退到简单的 host:port 格式
	parts := strings.Split(workerURL, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return ts.config.VectorHost, strconv.Itoa(ts.config.VectorTCPPort) // 使用配置的默认值
}
