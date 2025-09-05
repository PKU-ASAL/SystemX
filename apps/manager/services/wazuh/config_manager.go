package wazuh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sysarmor/sysarmor/apps/manager/config"
	"github.com/sysarmor/sysarmor/apps/manager/models"
	"gopkg.in/yaml.v3"
)

// ConfigManager Wazuh配置管理器
type ConfigManager struct {
	mu             sync.RWMutex
	staticConfig   *config.WazuhConfig
	dynamicConfig  *models.WazuhDynamicAuthRequest
	managerClient  *ManagerClient
	indexerClient  *IndexerClient
	lastUpdateTime time.Time
	configActive   bool
	configFilePath string
}

// NewConfigManager 创建新的配置管理器
func NewConfigManager(staticConfig *config.WazuhConfig, configFilePath string) *ConfigManager {
	return &ConfigManager{
		staticConfig:   staticConfig,
		configActive:   false,
		lastUpdateTime: time.Now(),
		configFilePath: configFilePath,
	}
}

// UpdateConfig 更新动态配置
func (cm *ConfigManager) UpdateConfig(ctx context.Context, req *models.WazuhDynamicAuthRequest) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 验证配置
	if err := cm.validateConfig(req); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	// 保存旧配置以便回滚
	oldConfig := cm.dynamicConfig
	oldManagerClient := cm.managerClient
	oldIndexerClient := cm.indexerClient

	// 更新配置
	cm.dynamicConfig = req
	cm.lastUpdateTime = time.Now()

	// 重新创建客户端
	if err := cm.recreateClients(); err != nil {
		// 回滚配置
		cm.dynamicConfig = oldConfig
		cm.managerClient = oldManagerClient
		cm.indexerClient = oldIndexerClient
		return fmt.Errorf("创建客户端失败: %w", err)
	}

	// 测试连接
	if err := cm.testConnections(ctx); err != nil {
		// 回滚配置
		cm.dynamicConfig = oldConfig
		cm.managerClient = oldManagerClient
		cm.indexerClient = oldIndexerClient
		return fmt.Errorf("连接测试失败: %w", err)
	}

	// 更新配置文件
	if err := cm.updateConfigFile(); err != nil {
		// 回滚配置
		cm.dynamicConfig = oldConfig
		cm.managerClient = oldManagerClient
		cm.indexerClient = oldIndexerClient
		return fmt.Errorf("更新配置文件失败: %w", err)
	}

	cm.configActive = true
	return nil
}

// GetCurrentConfig 获取当前配置（脱敏）
func (cm *ConfigManager) GetCurrentConfig() *models.WazuhConfigResponse {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	response := &models.WazuhConfigResponse{
		Status: models.WazuhConfigStatusInactive,
	}

	if cm.configActive && cm.dynamicConfig != nil {
		response.Status = models.WazuhConfigStatusActive

		if cm.dynamicConfig.Manager != nil {
			response.Manager = &models.WazuhManagerConfigInfo{
				URL:       cm.dynamicConfig.Manager.URL,
				Username:  cm.dynamicConfig.Manager.Username,
				Password:  models.MaskPassword(cm.dynamicConfig.Manager.Password),
				Timeout:   cm.dynamicConfig.Manager.Timeout,
				TLSVerify: cm.getBoolValue(cm.dynamicConfig.Manager.TLSVerify, false),
				Status:    models.WazuhConfigStatusActive,
			}
		}

		if cm.dynamicConfig.Indexer != nil {
			response.Indexer = &models.WazuhIndexerConfigInfo{
				URL:       cm.dynamicConfig.Indexer.URL,
				Username:  cm.dynamicConfig.Indexer.Username,
				Password:  models.MaskPassword(cm.dynamicConfig.Indexer.Password),
				Timeout:   cm.dynamicConfig.Indexer.Timeout,
				TLSVerify: cm.getBoolValue(cm.dynamicConfig.Indexer.TLSVerify, false),
				Status:    models.WazuhConfigStatusActive,
			}
		}
	} else {
		// 显示静态配置信息（脱敏）
		if cm.staticConfig.IsManagerEnabled() {
			managerConfig := cm.staticConfig.GetManagerConfig()
			response.Manager = &models.WazuhManagerConfigInfo{
				URL:       managerConfig.URL,
				Username:  managerConfig.Username,
				Password:  models.MaskPassword(managerConfig.Password),
				Timeout:   managerConfig.Timeout.String(),
				TLSVerify: managerConfig.TLSVerify,
				Status:    models.WazuhConfigStatusInactive,
			}
		}

		if cm.staticConfig.IsIndexerEnabled() {
			indexerConfig := cm.staticConfig.GetIndexerConfig()
			response.Indexer = &models.WazuhIndexerConfigInfo{
				URL:       indexerConfig.URL,
				Username:  indexerConfig.Username,
				Password:  models.MaskPassword(indexerConfig.Password),
				Timeout:   indexerConfig.Timeout.String(),
				TLSVerify: indexerConfig.TLSVerify,
				Status:    models.WazuhConfigStatusInactive,
			}
		}
	}

	return response
}

// TestConnection 测试连接
func (cm *ConfigManager) TestConnection(ctx context.Context, req *models.WazuhAuthTestRequest) (*models.WazuhAuthTestResponse, error) {
	response := &models.WazuhAuthTestResponse{
		Type:    req.Type,
		Results: make(map[string]models.WazuhTestResult),
		Overall: "success",
	}

	// 创建临时客户端进行测试
	tempConfig := req.Config
	if tempConfig == nil {
		cm.mu.RLock()
		tempConfig = cm.dynamicConfig
		cm.mu.RUnlock()
	}

	if tempConfig == nil {
		return nil, fmt.Errorf("没有可用的配置进行测试")
	}

	// 测试Manager连接
	if req.Type == models.WazuhAuthTestTypeManager || req.Type == models.WazuhAuthTestTypeBoth {
		if tempConfig.Manager != nil {
			result := cm.testManagerConnection(ctx, tempConfig.Manager)
			response.Results["manager"] = result
			if result.Status != "success" {
				response.Overall = "failed"
			}
		}
	}

	// 测试Indexer连接
	if req.Type == models.WazuhAuthTestTypeIndexer || req.Type == models.WazuhAuthTestTypeBoth {
		if tempConfig.Indexer != nil {
			result := cm.testIndexerConnection(ctx, tempConfig.Indexer)
			response.Results["indexer"] = result
			if result.Status != "success" {
				if response.Overall == "success" {
					response.Overall = "partial"
				} else {
					response.Overall = "failed"
				}
			}
		}
	}

	return response, nil
}

// GetManagerClient 获取Manager客户端
func (cm *ConfigManager) GetManagerClient() *ManagerClient {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.configActive && cm.managerClient != nil {
		return cm.managerClient
	}

	// 返回静态配置的客户端
	return NewManagerClient(cm.staticConfig)
}

// GetIndexerClient 获取Indexer客户端
func (cm *ConfigManager) GetIndexerClient() *IndexerClient {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.configActive && cm.indexerClient != nil {
		return cm.indexerClient
	}

	// 返回静态配置的客户端
	return NewIndexerClient(cm.staticConfig)
}

// IsManagerEnabled 检查Manager是否启用
func (cm *ConfigManager) IsManagerEnabled() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.configActive && cm.dynamicConfig != nil && cm.dynamicConfig.Manager != nil {
		return true
	}

	return cm.staticConfig.IsManagerEnabled()
}

// IsIndexerEnabled 检查Indexer是否启用
func (cm *ConfigManager) IsIndexerEnabled() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.configActive && cm.dynamicConfig != nil && cm.dynamicConfig.Indexer != nil {
		return true
	}

	return cm.staticConfig.IsIndexerEnabled()
}

// validateConfig 验证配置
func (cm *ConfigManager) validateConfig(req *models.WazuhDynamicAuthRequest) error {
	if req == nil {
		return fmt.Errorf("配置不能为空")
	}

	if req.Manager == nil && req.Indexer == nil {
		return fmt.Errorf("至少需要配置Manager或Indexer中的一个")
	}

	if req.Manager != nil {
		if req.Manager.URL == "" {
			return fmt.Errorf("Manager URL不能为空")
		}
		if req.Manager.Username == "" {
			return fmt.Errorf("Manager用户名不能为空")
		}
		if req.Manager.Password == "" {
			return fmt.Errorf("Manager密码不能为空")
		}
	}

	if req.Indexer != nil {
		if req.Indexer.URL == "" {
			return fmt.Errorf("Indexer URL不能为空")
		}
		if req.Indexer.Username == "" {
			return fmt.Errorf("Indexer用户名不能为空")
		}
		if req.Indexer.Password == "" {
			return fmt.Errorf("Indexer密码不能为空")
		}
	}

	return nil
}

// recreateClients 重新创建客户端
func (cm *ConfigManager) recreateClients() error {
	if cm.dynamicConfig == nil {
		return fmt.Errorf("动态配置为空")
	}

	// 创建临时配置
	tempConfig := cm.createTempConfig()

	// 创建新的客户端
	if cm.dynamicConfig.Manager != nil {
		cm.managerClient = NewManagerClient(tempConfig)
	}

	if cm.dynamicConfig.Indexer != nil {
		cm.indexerClient = NewIndexerClient(tempConfig)
	}

	return nil
}

// createTempConfig 创建临时配置
func (cm *ConfigManager) createTempConfig() *config.WazuhConfig {
	tempConfig := &config.WazuhConfig{
		Wazuh: config.WazuhSettings{
			Features: cm.staticConfig.Wazuh.Features,
		},
	}

	if cm.dynamicConfig.Manager != nil {
		timeout, _ := time.ParseDuration(cm.getStringValue(cm.dynamicConfig.Manager.Timeout, "30s"))
		tempConfig.Wazuh.Manager = config.WazuhManagerConfig{
			URL:       cm.dynamicConfig.Manager.URL,
			Username:  cm.dynamicConfig.Manager.Username,
			Password:  cm.dynamicConfig.Manager.Password,
			Timeout:   timeout,
			TLSVerify: cm.getBoolValue(cm.dynamicConfig.Manager.TLSVerify, false),
		}
		tempConfig.Wazuh.Features.ManagerEnabled = true
	}

	if cm.dynamicConfig.Indexer != nil {
		timeout, _ := time.ParseDuration(cm.getStringValue(cm.dynamicConfig.Indexer.Timeout, "30s"))
		tempConfig.Wazuh.Indexer = config.WazuhIndexerConfig{
			URL:       cm.dynamicConfig.Indexer.URL,
			Username:  cm.dynamicConfig.Indexer.Username,
			Password:  cm.dynamicConfig.Indexer.Password,
			Timeout:   timeout,
			TLSVerify: cm.getBoolValue(cm.dynamicConfig.Indexer.TLSVerify, false),
			Indices:   cm.staticConfig.Wazuh.Indexer.Indices,
		}
		tempConfig.Wazuh.Features.IndexerEnabled = true
	}

	return tempConfig
}

// testConnections 测试连接
func (cm *ConfigManager) testConnections(ctx context.Context) error {
	if cm.managerClient != nil {
		if err := cm.managerClient.HealthCheck(ctx); err != nil {
			return fmt.Errorf("Manager连接测试失败: %w", err)
		}
	}

	if cm.indexerClient != nil {
		if _, err := cm.indexerClient.HealthCheck(ctx); err != nil {
			return fmt.Errorf("Indexer连接测试失败: %w", err)
		}
	}

	return nil
}

// testManagerConnection 测试Manager连接
func (cm *ConfigManager) testManagerConnection(ctx context.Context, authConfig *models.WazuhManagerAuthConfig) models.WazuhTestResult {
	start := time.Now()

	// 创建临时配置和客户端
	timeout, _ := time.ParseDuration(cm.getStringValue(authConfig.Timeout, "30s"))
	tempConfig := &config.WazuhConfig{
		Wazuh: config.WazuhSettings{
			Manager: config.WazuhManagerConfig{
				URL:       authConfig.URL,
				Username:  authConfig.Username,
				Password:  authConfig.Password,
				Timeout:   timeout,
				TLSVerify: cm.getBoolValue(authConfig.TLSVerify, false),
			},
			Features: config.WazuhFeaturesConfig{
				ManagerEnabled: true,
			},
		},
	}

	client := NewManagerClient(tempConfig)
	err := client.HealthCheck(ctx)
	latency := time.Since(start)

	if err != nil {
		return models.WazuhTestResult{
			Status:  "failed",
			Message: err.Error(),
			Latency: latency.String(),
		}
	}

	return models.WazuhTestResult{
		Status:  "success",
		Message: "连接成功",
		Latency: latency.String(),
	}
}

// testIndexerConnection 测试Indexer连接
func (cm *ConfigManager) testIndexerConnection(ctx context.Context, authConfig *models.WazuhIndexerAuthConfig) models.WazuhTestResult {
	start := time.Now()

	// 创建临时配置和客户端
	timeout, _ := time.ParseDuration(cm.getStringValue(authConfig.Timeout, "30s"))
	tempConfig := &config.WazuhConfig{
		Wazuh: config.WazuhSettings{
			Indexer: config.WazuhIndexerConfig{
				URL:       authConfig.URL,
				Username:  authConfig.Username,
				Password:  authConfig.Password,
				Timeout:   timeout,
				TLSVerify: cm.getBoolValue(authConfig.TLSVerify, false),
				Indices:   cm.staticConfig.Wazuh.Indexer.Indices,
			},
			Features: config.WazuhFeaturesConfig{
				IndexerEnabled: true,
			},
		},
	}

	client := NewIndexerClient(tempConfig)
	_, err := client.HealthCheck(ctx)
	latency := time.Since(start)

	if err != nil {
		return models.WazuhTestResult{
			Status:  "failed",
			Message: err.Error(),
			Latency: latency.String(),
		}
	}

	return models.WazuhTestResult{
		Status:  "success",
		Message: "连接成功",
		Latency: latency.String(),
	}
}

// updateConfigFile 更新配置文件
func (cm *ConfigManager) updateConfigFile() error {
	if cm.configFilePath == "" {
		return fmt.Errorf("配置文件路径未设置")
	}

	// 创建备份文件路径
	backupPath := cm.configFilePath + ".backup." + fmt.Sprintf("%d", time.Now().Unix())

	// 读取现有配置文件的原始内容
	var originalData []byte

	if _, err := os.Stat(cm.configFilePath); err == nil {
		// 文件存在，读取现有内容
		originalData, err = os.ReadFile(cm.configFilePath)
		if err != nil {
			return fmt.Errorf("读取配置文件失败: %w", err)
		}

		// 创建备份
		if err := os.WriteFile(backupPath, originalData, 0644); err != nil {
			return fmt.Errorf("创建备份文件失败: %w", err)
		}
	} else {
		// 文件不存在，确保目录存在
		dir := filepath.Dir(cm.configFilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建配置目录失败: %w", err)
		}
		return fmt.Errorf("配置文件不存在: %s", cm.configFilePath)
	}

	var node yaml.Node
	if err := yaml.Unmarshal(originalData, &node); err != nil {
		return fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 递归更新指定字段
	if err := cm.updateYAMLNode(&node); err != nil {
		return fmt.Errorf("更新配置节点失败: %w", err)
	}

	// 序列化为YAML，保持原有结构和注释
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&node); err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	encoder.Close()

	// 写入临时文件
	tempPath := cm.configFilePath + ".tmp"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	if err := os.Rename(tempPath, cm.configFilePath); err != nil {
		// 清理临时文件
		os.Remove(tempPath)
		return fmt.Errorf("替换配置文件失败: %w", err)
	}

	return nil
}

// updateYAMLNode 递归更新YAML节点中的指定字段
func (cm *ConfigManager) updateYAMLNode(node *yaml.Node) error {
	if node.Kind != yaml.DocumentNode {
		return fmt.Errorf("期望文档节点")
	}

	if len(node.Content) == 0 {
		return fmt.Errorf("文档节点为空")
	}

	rootNode := node.Content[0]
	if rootNode.Kind != yaml.MappingNode {
		return fmt.Errorf("期望映射节点")
	}

	// 查找 wazuh 节点
	wazuhNode, err := cm.findMapValue(rootNode, "wazuh")
	if err != nil {
		return fmt.Errorf("找不到wazuh配置节点: %w", err)
	}

	// 更新 manager 配置
	if cm.dynamicConfig.Manager != nil {
		managerNode, err := cm.findMapValue(wazuhNode, "manager")
		if err != nil {
			return fmt.Errorf("找不到manager配置节点: %w", err)
		}

		if err := cm.updateMapValue(managerNode, "url", cm.dynamicConfig.Manager.URL); err != nil {
			return fmt.Errorf("更新manager url失败: %w", err)
		}
		if err := cm.updateMapValue(managerNode, "username", cm.dynamicConfig.Manager.Username); err != nil {
			return fmt.Errorf("更新manager username失败: %w", err)
		}
		if err := cm.updateMapValue(managerNode, "password", cm.dynamicConfig.Manager.Password); err != nil {
			return fmt.Errorf("更新manager password失败: %w", err)
		}
	}

	// 更新 indexer 配置
	if cm.dynamicConfig.Indexer != nil {
		indexerNode, err := cm.findMapValue(wazuhNode, "indexer")
		if err != nil {
			return fmt.Errorf("找不到indexer配置节点: %w", err)
		}

		if err := cm.updateMapValue(indexerNode, "url", cm.dynamicConfig.Indexer.URL); err != nil {
			return fmt.Errorf("更新indexer url失败: %w", err)
		}
		if err := cm.updateMapValue(indexerNode, "username", cm.dynamicConfig.Indexer.Username); err != nil {
			return fmt.Errorf("更新indexer username失败: %w", err)
		}
		if err := cm.updateMapValue(indexerNode, "password", cm.dynamicConfig.Indexer.Password); err != nil {
			return fmt.Errorf("更新indexer password失败: %w", err)
		}
	}

	return nil
}

// findMapValue 在映射节点中查找指定键的值节点
func (cm *ConfigManager) findMapValue(mapNode *yaml.Node, key string) (*yaml.Node, error) {
	if mapNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("节点不是映射类型")
	}

	for i := 0; i < len(mapNode.Content); i += 2 {
		keyNode := mapNode.Content[i]
		valueNode := mapNode.Content[i+1]

		if keyNode.Value == key {
			return valueNode, nil
		}
	}

	return nil, fmt.Errorf("找不到键: %s", key)
}

// updateMapValue 更新映射节点中指定键的值
func (cm *ConfigManager) updateMapValue(mapNode *yaml.Node, key, value string) error {
	if mapNode.Kind != yaml.MappingNode {
		return fmt.Errorf("节点不是映射类型")
	}

	for i := 0; i < len(mapNode.Content); i += 2 {
		keyNode := mapNode.Content[i]
		valueNode := mapNode.Content[i+1]

		if keyNode.Value == key {
			valueNode.Value = value
			return nil
		}
	}

	return fmt.Errorf("找不到键: %s", key)
}

// getBoolValue 获取布尔值，如果为nil则返回默认值
func (cm *ConfigManager) getBoolValue(ptr *bool, defaultValue bool) bool {
	if ptr == nil {
		return defaultValue
	}
	return *ptr
}

// getStringValue 获取字符串值，如果为空则返回默认值
func (cm *ConfigManager) getStringValue(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// ResetToStaticConfig 重置为静态配置
func (cm *ConfigManager) ResetToStaticConfig() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.dynamicConfig = nil
	cm.managerClient = nil
	cm.indexerClient = nil
	cm.configActive = false
	cm.lastUpdateTime = time.Now()
}
