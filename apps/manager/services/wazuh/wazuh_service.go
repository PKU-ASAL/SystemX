package wazuh

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sysarmor/sysarmor/apps/manager/config"
	"github.com/sysarmor/sysarmor/apps/manager/models"
)

// WazuhService Wazuh服务 (遵循现有服务模式)
type WazuhService struct {
	config        *config.WazuhConfig
	configManager *ConfigManager
	managerClient *ManagerClient
	indexerClient *IndexerClient
	enabled       bool
}

// NewWazuhService 创建Wazuh服务 (遵循现有构造函数模式)
func NewWazuhService(cfg *config.Config) (*WazuhService, error) {
	// 暂时返回一个禁用的服务，因为配置系统还未完全集成
	fmt.Printf("🔌 Wazuh service disabled (configuration not yet integrated)")
	return &WazuhService{enabled: false}, nil
}

// IsEnabled 检查服务是否启用
func (s *WazuhService) IsEnabled() bool {
	return s.enabled
}

// IsManagerEnabled 检查Manager是否启用
func (s *WazuhService) IsManagerEnabled() bool {
	if !s.enabled {
		return false
	}
	return s.configManager.IsManagerEnabled()
}

// IsIndexerEnabled 检查Indexer是否启用
func (s *WazuhService) IsIndexerEnabled() bool {
	if !s.enabled {
		return false
	}
	return s.configManager.IsIndexerEnabled()
}

// GetManagerClient 获取Manager客户端
func (s *WazuhService) GetManagerClient() *ManagerClient {
	if !s.enabled {
		return nil
	}
	return s.configManager.GetManagerClient()
}

// GetIndexerClient 获取Indexer客户端
func (s *WazuhService) GetIndexerClient() *IndexerClient {
	if !s.enabled {
		return nil
	}
	return s.configManager.GetIndexerClient()
}

// GetConfigManager 获取配置管理器
func (s *WazuhService) GetConfigManager() *ConfigManager {
	if !s.enabled {
		return nil
	}
	return s.configManager
}

// HealthCheck 健康检查
func (s *WazuhService) HealthCheck(ctx context.Context) error {
	if !s.enabled {
		return nil // 服务未启用，跳过检查
	}

	var errors []string

	// 检查Manager连接
	if s.IsManagerEnabled() {
		if client := s.GetManagerClient(); client != nil {
			if err := client.HealthCheck(ctx); err != nil {
				errors = append(errors, fmt.Sprintf("wazuh manager unhealthy: %v", err))
			}
		}
	}

	// 检查Indexer连接
	if s.IsIndexerEnabled() {
		if client := s.GetIndexerClient(); client != nil {
			if _, err := client.HealthCheck(ctx); err != nil {
				errors = append(errors, fmt.Sprintf("wazuh indexer unhealthy: %v", err))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("wazuh health check failed: %v", errors)
	}

	return nil
}

// GetConfig 获取配置
func (s *WazuhService) GetConfig() *models.WazuhConfigResponse {
	if !s.enabled {
		return &models.WazuhConfigResponse{
			Status:  models.WazuhConfigStatusInactive,
			Message: "Wazuh service is disabled",
		}
	}
	return s.configManager.GetCurrentConfig()
}

// UpdateConfig 更新配置
func (s *WazuhService) UpdateConfig(ctx context.Context, req map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 将map[string]interface{}转换为WazuhDynamicAuthRequest
	dynamicReq := &models.WazuhDynamicAuthRequest{}
	
	// 解析Manager配置
	if managerData, ok := req["manager"].(map[string]interface{}); ok {
		dynamicReq.Manager = &models.WazuhManagerAuthConfig{}
		if url, ok := managerData["url"].(string); ok {
			dynamicReq.Manager.URL = url
		}
		if username, ok := managerData["username"].(string); ok {
			dynamicReq.Manager.Username = username
		}
		if password, ok := managerData["password"].(string); ok {
			dynamicReq.Manager.Password = password
		}
		if timeout, ok := managerData["timeout"].(string); ok {
			dynamicReq.Manager.Timeout = timeout
		}
		if tlsVerify, ok := managerData["tls_verify"].(bool); ok {
			dynamicReq.Manager.TLSVerify = &tlsVerify
		}
	}
	
	// 解析Indexer配置
	if indexerData, ok := req["indexer"].(map[string]interface{}); ok {
		dynamicReq.Indexer = &models.WazuhIndexerAuthConfig{}
		if url, ok := indexerData["url"].(string); ok {
			dynamicReq.Indexer.URL = url
		}
		if username, ok := indexerData["username"].(string); ok {
			dynamicReq.Indexer.Username = username
		}
		if password, ok := indexerData["password"].(string); ok {
			dynamicReq.Indexer.Password = password
		}
		if timeout, ok := indexerData["timeout"].(string); ok {
			dynamicReq.Indexer.Timeout = timeout
		}
		if tlsVerify, ok := indexerData["tls_verify"].(bool); ok {
			dynamicReq.Indexer.TLSVerify = &tlsVerify
		}
	}
	
	return s.configManager.UpdateConfig(ctx, dynamicReq)
}

// ValidateConfig 验证配置
func (s *WazuhService) ValidateConfig(ctx context.Context, req map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 构建测试请求
	testReq := &models.WazuhAuthTestRequest{
		Type: models.WazuhAuthTestTypeBoth,
	}
	
	// 将map转换为配置结构
	dynamicReq := &models.WazuhDynamicAuthRequest{}
	
	if managerData, ok := req["manager"].(map[string]interface{}); ok {
		dynamicReq.Manager = &models.WazuhManagerAuthConfig{}
		if url, ok := managerData["url"].(string); ok {
			dynamicReq.Manager.URL = url
		}
		if username, ok := managerData["username"].(string); ok {
			dynamicReq.Manager.Username = username
		}
		if password, ok := managerData["password"].(string); ok {
			dynamicReq.Manager.Password = password
		}
	}
	
	if indexerData, ok := req["indexer"].(map[string]interface{}); ok {
		dynamicReq.Indexer = &models.WazuhIndexerAuthConfig{}
		if url, ok := indexerData["url"].(string); ok {
			dynamicReq.Indexer.URL = url
		}
		if username, ok := indexerData["username"].(string); ok {
			dynamicReq.Indexer.Username = username
		}
		if password, ok := indexerData["password"].(string); ok {
			dynamicReq.Indexer.Password = password
		}
	}
	
	testReq.Config = dynamicReq
	
	// 执行连接测试
	result, err := s.configManager.TestConnection(ctx, testReq)
	if err != nil {
		return err
	}
	
	if result.Overall != "success" {
		return fmt.Errorf("configuration validation failed: %s", result.Overall)
	}
	
	return nil
}

// ReloadConfig 重新加载配置
func (s *WazuhService) ReloadConfig(ctx context.Context) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 重置为静态配置
	s.configManager.ResetToStaticConfig()
	return nil
}

// GetManagerInfo 获取Manager信息
func (s *WazuhService) GetManagerInfo(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetManagerInfo(ctx)
}

// GetManagerStatus 获取Manager状态
func (s *WazuhService) GetManagerStatus(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetManagerStatus(ctx)
}

// GetManagerLogs 获取Manager日志
func (s *WazuhService) GetManagerLogs(ctx context.Context, offset, limit, level, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	params := map[string]interface{}{
		"offset": parseInt(offset),
		"limit":  parseInt(limit),
	}
	
	if level != "" {
		params["level"] = level
	}
	if search != "" {
		params["search"] = search
	}
	
	return client.GetManagerLogs(ctx, params)
}

// GetManagerStats 获取Manager统计
func (s *WazuhService) GetManagerStats(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetManagerStats(ctx)
}

// RestartManager 重启Manager
func (s *WazuhService) RestartManager(ctx context.Context) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	_, err := client.RestartManager(ctx)
	return err
}

// GetManagerConfiguration 获取Manager配置
func (s *WazuhService) GetManagerConfiguration(ctx context.Context, section string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetManagerConfiguration(ctx, section)
}

// UpdateManagerConfiguration 更新Manager配置
func (s *WazuhService) UpdateManagerConfiguration(ctx context.Context, req map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	return client.UpdateManagerConfiguration(ctx, req)
}

// GetAgents 获取Agent列表
func (s *WazuhService) GetAgents(ctx context.Context, offset, limit, sort, search, status string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	// 转换参数
	params := &WazuhAgentParams{
		Offset: parseInt(offset),
		Limit:  parseInt(limit),
		Sort:   sort,
		Search: search,
		Status: status,
	}
	
	return client.GetAgents(ctx, params)
}

// AddAgent 添加Agent
func (s *WazuhService) AddAgent(ctx context.Context, req *models.WazuhAgent) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	// 转换请求格式
	addReq := &models.WazuhAddAgentRequest{
		Name: req.Name,
		IP:   req.IP,
	}
	
	return client.AddAgent(ctx, addReq)
}

// GetAgent 获取单个Agent
func (s *WazuhService) GetAgent(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetAgent(ctx, agentID)
}

// UpdateAgent 更新Agent
func (s *WazuhService) UpdateAgent(ctx context.Context, agentID string, req *models.WazuhAgent) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	// 构建更新请求
	updateReq := &models.WazuhUpdateAgentRequest{
		Name: req.Name,
		IP:   req.IP,
	}
	
	_, err := client.UpdateAgent(ctx, agentID, updateReq)
	return err
}

// DeleteAgent 删除Agent
func (s *WazuhService) DeleteAgent(ctx context.Context, agentID string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	_, err := client.DeleteAgent(ctx, agentID, false)
	return err
}

// RestartAgent 重启Agent
func (s *WazuhService) RestartAgent(ctx context.Context, agentID string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	_, err := client.RestartAgent(ctx, agentID)
	return err
}

// GetAgentKey 获取Agent密钥
func (s *WazuhService) GetAgentKey(ctx context.Context, agentID string) (string, error) {
	if !s.enabled {
		return "", fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return "", fmt.Errorf("wazuh manager client not available")
	}
	
	keyResp, err := client.GetAgentKey(ctx, agentID)
	if err != nil {
		return "", err
	}
	
	if len(keyResp.Data.AffectedItems) > 0 {
		return keyResp.Data.AffectedItems[0].Key, nil
	}
	
	return "", fmt.Errorf("no key found for agent %s", agentID)
}

// UpgradeAgent 升级Agent
func (s *WazuhService) UpgradeAgent(ctx context.Context, agentID, version string, force bool) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	// 构建升级请求
	upgradeReq := &models.WazuhUpgradeRequest{
		AgentsList: []string{agentID},
		Force:      force,
	}
	
	if version != "" {
		upgradeReq.UpgradeVersion = version
	}
	
	_, err := client.UpgradeAgents(ctx, upgradeReq)
	return err
}

// GetAgentConfig 获取Agent配置
func (s *WazuhService) GetAgentConfig(ctx context.Context, agentID, section string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetAgentConfiguration(ctx, agentID, section)
}

// UpdateAgentConfig 更新Agent配置
func (s *WazuhService) UpdateAgentConfig(ctx context.Context, agentID string, req map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	return client.UpdateAgentConfiguration(ctx, agentID, req)
}

// GetSystemInfo 获取系统信息
func (s *WazuhService) GetSystemInfo(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetSystemInfo(ctx, agentID)
}

// GetHardwareInfo 获取硬件信息
func (s *WazuhService) GetHardwareInfo(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetHardwareInfo(ctx, agentID)
}

// GetNetworkAddresses 获取网络地址信息
func (s *WazuhService) GetNetworkAddresses(ctx context.Context, agentID string, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	params := &WazuhNetworkParams{
		Offset: parseInt(offset),
		Limit:  parseInt(limit),
		Sort:   sort,
		Search: search,
	}
	
	return client.GetNetworkAddresses(ctx, agentID, params)
}

// GetProcesses 获取进程信息
func (s *WazuhService) GetProcesses(ctx context.Context, agentID string, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	params := &WazuhProcessParams{
		Offset: parseInt(offset),
		Limit:  parseInt(limit),
		Sort:   sort,
		Search: search,
	}
	
	return client.GetProcesses(ctx, agentID, params)
}

// GetPackages 获取软件包信息
func (s *WazuhService) GetPackages(ctx context.Context, agentID string, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	params := &WazuhPackageParams{
		Offset: parseInt(offset),
		Limit:  parseInt(limit),
		Sort:   sort,
		Search: search,
	}
	
	return client.GetPackages(ctx, agentID, params)
}

// GetPorts 获取端口信息
func (s *WazuhService) GetPorts(ctx context.Context, agentID string, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	params := &WazuhPortParams{
		Offset: parseInt(offset),
		Limit:  parseInt(limit),
		Sort:   sort,
		Search: search,
	}
	
	return client.GetPorts(ctx, agentID, params)
}

// GetHotfixes 获取热修复信息
func (s *WazuhService) GetHotfixes(ctx context.Context, agentID string, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	params := &WazuhHotfixParams{
		Offset: parseInt(offset),
		Limit:  parseInt(limit),
		Sort:   sort,
		Search: search,
	}
	
	return client.GetHotfixes(ctx, agentID, params)
}

// GetNetworkProtocols 获取网络协议信息
func (s *WazuhService) GetNetworkProtocols(ctx context.Context, agentID string, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	params := &WazuhNetworkParams{
		Offset: parseInt(offset),
		Limit:  parseInt(limit),
		Sort:   sort,
		Search: search,
	}
	
	return client.GetNetworkProtocols(ctx, agentID, params)
}

// GetAgentStats 获取代理统计信息
func (s *WazuhService) GetAgentStats(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetAgentStats(ctx, agentID)
}

// GetLogcollectorStats 获取日志收集器统计信息
func (s *WazuhService) GetLogcollectorStats(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetLogcollectorStats(ctx, agentID)
}

// GetAgentDaemonStats 获取代理守护进程统计信息
func (s *WazuhService) GetAgentDaemonStats(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetAgentDaemonStats(ctx, agentID)
}

// UpgradeAgents 升级多个代理
func (s *WazuhService) UpgradeAgents(ctx context.Context, req *models.WazuhUpgradeRequest) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.UpgradeAgents(ctx, req)
}

// CustomUpgradeAgents 自定义升级代理
func (s *WazuhService) CustomUpgradeAgents(ctx context.Context, req *models.WazuhCustomUpgradeRequest) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.CustomUpgradeAgents(ctx, req)
}

// GetUpgradeResult 获取升级结果
func (s *WazuhService) GetUpgradeResult(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetUpgradeResult(ctx, []string{agentID})
}

// GetCiscatResults 获取CIS-CAT扫描结果
func (s *WazuhService) GetCiscatResults(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetCiscatResults(ctx, agentID)
}

// GetSCAResults 获取SCA扫描结果
func (s *WazuhService) GetSCAResults(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetSCAResults(ctx, agentID)
}

// GetRootcheckResults 获取Rootcheck扫描结果
func (s *WazuhService) GetRootcheckResults(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	params := &WazuhAgentParams{
		Limit: 100, // 默认限制
	}
	
	return client.GetRootcheckResults(ctx, agentID, params)
}

// ClearRootcheckResults 清除Rootcheck扫描结果
func (s *WazuhService) ClearRootcheckResults(ctx context.Context, agentID string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	_, err := client.ClearRootcheckResults(ctx, agentID)
	return err
}

// GetRootcheckLastScan 获取Rootcheck最后扫描时间
func (s *WazuhService) GetRootcheckLastScan(ctx context.Context, agentID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetRootcheckLastScan(ctx, agentID)
}

// RunRootcheck 运行Rootcheck扫描
func (s *WazuhService) RunRootcheck(ctx context.Context, agentsList []string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.RunRootcheck(ctx, agentsList)
}

// ExecuteActiveResponse 执行主动响应
func (s *WazuhService) ExecuteActiveResponse(ctx context.Context, req *models.WazuhActiveResponseRequest) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.ExecuteActiveResponse(ctx, req)
}

// GetOverviewAgents 获取代理概览信息
func (s *WazuhService) GetOverviewAgents(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetOverviewAgents(ctx)
}

// GetIndexerHealth 获取Indexer健康状态
func (s *WazuhService) GetIndexerHealth(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.HealthCheck(ctx)
}

// GetIndexerInfo 获取Indexer信息
func (s *WazuhService) GetIndexerInfo(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.GetClusterInfo(ctx)
}

// SearchEvents 搜索事件
func (s *WazuhService) SearchEvents(ctx context.Context, query *models.WazuhSearchQuery) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.SearchAlerts(ctx, query)
}

// GetEventByID 根据ID获取事件
func (s *WazuhService) GetEventByID(ctx context.Context, indexType, eventID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.GetEventByID(ctx, indexType, eventID)
}

// GetAggregations 获取聚合统计
func (s *WazuhService) GetAggregations(ctx context.Context, query *models.WazuhAggregationQuery) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.GetAggregations(ctx, query)
}

// GetIndices 获取索引列表
func (s *WazuhService) GetIndices(ctx context.Context, pattern string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.GetIndices(ctx, pattern)
}

// GetClusterHealth 获取集群健康状态
func (s *WazuhService) GetClusterHealth(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.HealthCheck(ctx)
}

// GetGroups 获取组列表
func (s *WazuhService) GetGroups(ctx context.Context, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	// 转换参数
	params := &WazuhGroupParams{
		Offset: parseInt(offset),
		Limit:  parseInt(limit),
		Sort:   sort,
		Search: search,
	}
	
	return client.GetGroups(ctx, params)
}

// CreateGroup 创建组
func (s *WazuhService) CreateGroup(ctx context.Context, name string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	_, err := client.CreateGroup(ctx, name)
	return err
}

// GetGroup 获取单个组
func (s *WazuhService) GetGroup(ctx context.Context, name string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	// 通过组列表查询特定组
	params := &WazuhGroupParams{
		Search: name,
		Limit:  1,
	}
	
	return client.GetGroups(ctx, params)
}

// UpdateGroup 更新组
func (s *WazuhService) UpdateGroup(ctx context.Context, name string, req map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// Wazuh Manager API不直接支持组更新，通常通过配置文件管理
	return fmt.Errorf("group update not supported by Wazuh Manager API")
}

// DeleteGroup 删除组
func (s *WazuhService) DeleteGroup(ctx context.Context, name string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// Wazuh Manager API不直接支持组删除，需要通过文件系统操作
	return fmt.Errorf("group deletion not supported by Wazuh Manager API")
}

// AddAgentToGroup 添加Agent到组
func (s *WazuhService) AddAgentToGroup(ctx context.Context, groupName, agentID string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	_, err := client.AssignAgentToGroup(ctx, agentID, groupName)
	return err
}

// RemoveAgentFromGroup 从组中移除Agent
func (s *WazuhService) RemoveAgentFromGroup(ctx context.Context, groupName, agentID string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 将Agent分配到default组来实现从当前组移除
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	_, err := client.AssignAgentToGroup(ctx, agentID, "default")
	return err
}

// UpdateGroupConfiguration 更新组配置
func (s *WazuhService) UpdateGroupConfiguration(ctx context.Context, name string, req map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 组配置更新通常通过文件上传实现，这里暂不实现
	return fmt.Errorf("group configuration update requires file upload, not yet implemented")
}

// GetGroupAgents 获取组内Agent (支持两种调用方式)
func (s *WazuhService) GetGroupAgents(ctx context.Context, groupID string, params ...string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	// 解析可选参数
	var offset, limit, sort, search string
	if len(params) > 0 {
		offset = params[0]
	}
	if len(params) > 1 {
		limit = params[1]
	}
	if len(params) > 2 {
		sort = params[2]
	}
	if len(params) > 3 {
		search = params[3]
	}
	
	agentParams := &WazuhAgentParams{
		Offset: parseInt(offset),
		Limit:  parseInt(limit),
		Sort:   sort,
		Search: search,
	}
	
	return client.GetGroupAgents(ctx, groupID, agentParams)
}

// GetGroupConfiguration 获取组配置
func (s *WazuhService) GetGroupConfiguration(ctx context.Context, groupID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetGroupConfiguration(ctx, groupID)
}

// GetGroupFiles 获取组文件列表
func (s *WazuhService) GetGroupFiles(ctx context.Context, groupID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetGroupFiles(ctx, groupID)
}

// GetGroupFile 获取组文件内容
func (s *WazuhService) GetGroupFile(ctx context.Context, groupID, filename string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetGroupFile(ctx, groupID, filename)
}

// RestartGroupAgents 重启组内代理
func (s *WazuhService) RestartGroupAgents(ctx context.Context, groupID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.RestartGroupAgents(ctx, groupID)
}

// AssignAgentToGroup 将代理分配到组
func (s *WazuhService) AssignAgentToGroup(ctx context.Context, agentID, groupID string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return fmt.Errorf("wazuh manager client not available")
	}
	
	_, err := client.AssignAgentToGroup(ctx, agentID, groupID)
	return err
}

// GetAgentsSummary 获取Agent摘要
func (s *WazuhService) GetAgentsSummary(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	return client.GetAgentsSummary(ctx)
}

// GetMonitoringOverview 获取监控概览
func (s *WazuhService) GetMonitoringOverview(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	overview := make(map[string]interface{})
	
	// 获取Manager状态
	if s.IsManagerEnabled() {
		if client := s.GetManagerClient(); client != nil {
			if managerInfo, err := client.GetManagerInfo(ctx); err == nil {
				overview["manager_info"] = managerInfo
			}
			if managerStatus, err := client.GetManagerStatus(ctx); err == nil {
				overview["manager_status"] = managerStatus
			}
			if agentsSummary, err := client.GetAgentsSummary(ctx); err == nil {
				overview["agents_summary"] = agentsSummary
			}
		}
	}
	
	// 获取Indexer状态
	if s.IsIndexerEnabled() {
		if client := s.GetIndexerClient(); client != nil {
			if indexerHealth, err := client.HealthCheck(ctx); err == nil {
				overview["indexer_health"] = indexerHealth
			}
			if clusterInfo, err := client.GetClusterInfo(ctx); err == nil {
				overview["cluster_info"] = clusterInfo
			}
		}
	}
	
	return overview, nil
}

// SearchAlerts 搜索告警
func (s *WazuhService) SearchAlerts(ctx context.Context, query *models.WazuhSearchQuery) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.SearchAlerts(ctx, query)
}

// GetAlertsByAgent 根据Agent获取告警
func (s *WazuhService) GetAlertsByAgent(ctx context.Context, agentID string, limit int) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.GetAlertsByAgent(ctx, agentID, limit)
}

// GetAlertsByRule 根据规则获取告警
func (s *WazuhService) GetAlertsByRule(ctx context.Context, ruleID string, limit int) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.GetAlertsByRule(ctx, ruleID, limit)
}

// GetAlertsByLevel 根据级别获取告警
func (s *WazuhService) GetAlertsByLevel(ctx context.Context, level, limit int) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.GetAlertsByLevel(ctx, level, limit)
}

// AggregateAlerts 聚合告警统计
func (s *WazuhService) AggregateAlerts(ctx context.Context, aggType, field string, size int) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.AggregateAlerts(ctx, aggType, field, size)
}

// CreateIndex 创建索引
func (s *WazuhService) CreateIndex(ctx context.Context, name string, settings map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.CreateIndex(ctx, name, settings)
}

// DeleteIndex 删除索引
func (s *WazuhService) DeleteIndex(ctx context.Context, name string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.DeleteIndex(ctx, name)
}

// GetIndexTemplates 获取索引模板
func (s *WazuhService) GetIndexTemplates(ctx context.Context, pattern string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.GetIndexTemplates(ctx, pattern)
}

// CreateIndexTemplate 创建索引模板
func (s *WazuhService) CreateIndexTemplate(ctx context.Context, name string, template map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetIndexerClient()
	if client == nil {
		return fmt.Errorf("wazuh indexer client not available")
	}
	
	return client.CreateIndexTemplate(ctx, name, template)
}

// GetAlertStats 获取告警统计
func (s *WazuhService) GetAlertStats(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 实现告警统计逻辑，聚合多个查询结果
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	// 聚合不同级别的告警统计
	levelAgg := &models.WazuhAggregationQuery{
		IndexType: "alerts",
		GroupBy:   "rule.level",
		Size:      20,
	}
	
	return client.GetAggregations(ctx, levelAgg)
}

// GetAlertsSummary 获取告警摘要
func (s *WazuhService) GetAlertsSummary(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 实现告警摘要统计
	client := s.GetIndexerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh indexer client not available")
	}
	
	// 聚合代理告警统计
	agentAgg := &models.WazuhAggregationQuery{
		IndexType: "alerts",
		GroupBy:   "agent.id",
		Size:      50,
	}
	
	return client.GetAggregations(ctx, agentAgg)
}

// GetSystemStats 获取系统统计
func (s *WazuhService) GetSystemStats(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 聚合系统级别的统计信息
	stats := make(map[string]interface{})
	
	// 获取Manager统计
	if s.IsManagerEnabled() {
		if client := s.GetManagerClient(); client != nil {
			if managerStats, err := client.GetManagerStats(ctx); err == nil {
				stats["manager_stats"] = managerStats
			}
			if agentsSummary, err := client.GetAgentsSummary(ctx); err == nil {
				stats["agents_summary"] = agentsSummary
			}
		}
	}
	
	// 获取Indexer统计
	if s.IsIndexerEnabled() {
		if client := s.GetIndexerClient(); client != nil {
			if indexerHealth, err := client.HealthCheck(ctx); err == nil {
				stats["indexer_health"] = indexerHealth
			}
		}
	}
	
	return stats, nil
}

// GetRules 获取规则列表
func (s *WazuhService) GetRules(ctx context.Context, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	params := map[string]interface{}{
		"offset": parseInt(offset),
		"limit":  parseInt(limit),
	}
	
	if sort != "" {
		params["sort"] = sort
	}
	if search != "" {
		params["search"] = search
	}
	
	return client.GetRules(ctx, params)
}

// GetRule 获取单个规则
func (s *WazuhService) GetRule(ctx context.Context, ruleID string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	client := s.GetManagerClient()
	if client == nil {
		return nil, fmt.Errorf("wazuh manager client not available")
	}
	
	// 通过规则列表查询特定规则
	params := map[string]interface{}{
		"rule_ids": ruleID,
		"limit":    1,
	}
	
	return client.GetRules(ctx, params)
}

// CreateRule 创建规则
func (s *WazuhService) CreateRule(ctx context.Context, req map[string]interface{}) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 规则创建通常通过文件上传实现
	return nil, fmt.Errorf("rule creation requires file upload, not yet implemented")
}

// UpdateRule 更新规则
func (s *WazuhService) UpdateRule(ctx context.Context, ruleID string, req map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 规则更新通常通过文件上传实现
	return fmt.Errorf("rule update requires file upload, not yet implemented")
}

// DeleteRule 删除规则
func (s *WazuhService) DeleteRule(ctx context.Context, ruleID string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 规则删除通常通过文件操作实现
	return fmt.Errorf("rule deletion requires file operations, not yet implemented")
}

// GetRuleFiles 获取规则文件列表
func (s *WazuhService) GetRuleFiles(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 规则文件列表查询需要文件系统API
	return nil, fmt.Errorf("rule files listing requires file system API, not yet implemented")
}

// GetRuleFile 获取规则文件内容
func (s *WazuhService) GetRuleFile(ctx context.Context, filename string) (string, error) {
	if !s.enabled {
		return "", fmt.Errorf("wazuh service is disabled")
	}
	
	// 规则文件内容查询需要文件系统API
	return "", fmt.Errorf("rule file content requires file system API, not yet implemented")
}

// UpdateRuleFile 更新规则文件
func (s *WazuhService) UpdateRuleFile(ctx context.Context, filename, content string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 规则文件更新需要文件上传API
	return fmt.Errorf("rule file update requires file upload API, not yet implemented")
}

// GetDecoders 获取解码器列表
func (s *WazuhService) GetDecoders(ctx context.Context, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 解码器列表查询需要Manager API支持
	return nil, fmt.Errorf("decoder listing requires Manager API support, not yet implemented")
}

// GetDecoder 获取单个解码器
func (s *WazuhService) GetDecoder(ctx context.Context, name string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 单个解码器查询需要Manager API支持
	return nil, fmt.Errorf("decoder query requires Manager API support, not yet implemented")
}

// CreateDecoder 创建解码器
func (s *WazuhService) CreateDecoder(ctx context.Context, req map[string]interface{}) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 解码器创建通常通过文件上传实现
	return nil, fmt.Errorf("decoder creation requires file upload, not yet implemented")
}

// UpdateDecoder 更新解码器
func (s *WazuhService) UpdateDecoder(ctx context.Context, name string, req map[string]interface{}) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 解码器更新通常通过文件上传实现
	return fmt.Errorf("decoder update requires file upload, not yet implemented")
}

// DeleteDecoder 删除解码器
func (s *WazuhService) DeleteDecoder(ctx context.Context, name string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 解码器删除通常通过文件操作实现
	return fmt.Errorf("decoder deletion requires file operations, not yet implemented")
}

// GetDecoderFiles 获取解码器文件列表
func (s *WazuhService) GetDecoderFiles(ctx context.Context) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 解码器文件列表查询需要文件系统API
	return nil, fmt.Errorf("decoder files listing requires file system API, not yet implemented")
}

// GetDecoderFile 获取解码器文件内容
func (s *WazuhService) GetDecoderFile(ctx context.Context, filename string) (string, error) {
	if !s.enabled {
		return "", fmt.Errorf("wazuh service is disabled")
	}
	
	// 解码器文件内容查询需要文件系统API
	return "", fmt.Errorf("decoder file content requires file system API, not yet implemented")
}

// UpdateDecoderFile 更新解码器文件
func (s *WazuhService) UpdateDecoderFile(ctx context.Context, filename, content string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// 解码器文件更新需要文件上传API
	return fmt.Errorf("decoder file update requires file upload API, not yet implemented")
}

// GetLists 获取CDB列表
func (s *WazuhService) GetLists(ctx context.Context, offset, limit, sort, search string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// CDB列表查询需要Manager API支持
	return nil, fmt.Errorf("CDB lists query requires Manager API support, not yet implemented")
}

// GetList 获取单个CDB列表
func (s *WazuhService) GetList(ctx context.Context, filename string) (interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("wazuh service is disabled")
	}
	
	// 单个CDB列表查询需要Manager API支持
	return nil, fmt.Errorf("CDB list query requires Manager API support, not yet implemented")
}

// CreateList 创建CDB列表
func (s *WazuhService) CreateList(ctx context.Context, filename, content string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// CDB列表创建通常通过文件上传实现
	return fmt.Errorf("CDB list creation requires file upload, not yet implemented")
}

// UpdateList 更新CDB列表
func (s *WazuhService) UpdateList(ctx context.Context, filename, content string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// CDB列表更新通常通过文件上传实现
	return fmt.Errorf("CDB list update requires file upload, not yet implemented")
}

// DeleteList 删除CDB列表
func (s *WazuhService) DeleteList(ctx context.Context, filename string) error {
	if !s.enabled {
		return fmt.Errorf("wazuh service is disabled")
	}
	
	// CDB列表删除通常通过文件操作实现
	return fmt.Errorf("CDB list deletion requires file operations, not yet implemented")
}

// WazuhGroupParams 组查询参数
type WazuhGroupParams struct {
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Sort   string `json:"sort,omitempty"`
	Search string `json:"search,omitempty"`
}

// parseInt 辅助函数：将字符串转换为整数
func parseInt(s string) int {
	if s == "" {
		return 0
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}
