package wazuh

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/sysarmor/sysarmor/apps/manager/config"
	"github.com/sysarmor/sysarmor/apps/manager/models"
)

// ManagerClient Wazuh Manager API客户端
type ManagerClient struct {
	config     *config.WazuhConfig
	httpClient *http.Client
	token      string
	tokenExp   time.Time
}

// NewManagerClient 创建新的Wazuh Manager客户端
func NewManagerClient(cfg *config.WazuhConfig) *ManagerClient {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !cfg.Wazuh.Manager.TLSVerify,
		},
	}

	return &ManagerClient{
		config: cfg,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   cfg.Wazuh.Manager.Timeout,
		},
	}
}

// authenticate 获取JWT认证令牌
func (c *ManagerClient) authenticate(ctx context.Context) error {
	// 检查现有token是否仍然有效
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return nil
	}

	// 构建认证请求
	authURL := fmt.Sprintf("%s/security/user/authenticate", c.config.Wazuh.Manager.URL)

	req, err := http.NewRequestWithContext(ctx, "POST", authURL, nil)
	if err != nil {
		return fmt.Errorf("创建认证请求失败: %w", err)
	}

	// 设置基本认证
	req.SetBasicAuth(c.config.Wazuh.Manager.Username, c.config.Wazuh.Manager.Password)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("认证请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("认证失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var authResp struct {
		Data struct {
			Token     string `json:"token"`
			ExpiresIn int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return fmt.Errorf("解析认证响应失败: %w", err)
	}

	// 保存token和过期时间
	c.token = authResp.Data.Token
	c.tokenExp = time.Now().Add(time.Duration(authResp.Data.ExpiresIn) * time.Second)

	return nil
}

// makeRequest 发送带认证的HTTP请求
func (c *ManagerClient) makeRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Response, error) {
	// 确保已认证
	if err := c.authenticate(ctx); err != nil {
		return nil, fmt.Errorf("认证失败: %w", err)
	}

	// 构建完整URL
	fullURL := fmt.Sprintf("%s%s", c.config.Wazuh.Manager.URL, endpoint)

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	return resp, nil
}

// GetAgents 获取代理列表
func (c *ManagerClient) GetAgents(ctx context.Context, params *WazuhAgentParams) (*models.WazuhAgentListResponse, error) {
	// 构建查询参数
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
		if params.Status != "" {
			query.Set("status", params.Status)
		}
		if params.OS != "" {
			query.Set("os.platform", params.OS)
		}
		if params.Version != "" {
			query.Set("version", params.Version)
		}
		if params.Group != "" {
			query.Set("group", params.Group)
		}
	}

	endpoint := "/agents"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取代理列表失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhAgentListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析代理列表响应失败: %w", err)
	}

	return &result, nil
}

// UpgradeAgents 升级代理
func (c *ManagerClient) UpgradeAgents(ctx context.Context, req *models.WazuhUpgradeRequest) (*models.WazuhUpgradeResponse, error) {
	query := url.Values{}
	for _, agentID := range req.AgentsList {
		query.Add("agents_list", agentID)
	}

	if req.WPKRepo != "" {
		query.Set("wpk_repo", req.WPKRepo)
	}
	if req.UpgradeVersion != "" {
		query.Set("upgrade_version", req.UpgradeVersion)
	}
	if req.Force {
		query.Set("force", "true")
	}

	endpoint := "/agents/upgrade?" + query.Encode()

	resp, err := c.makeRequest(ctx, "PUT", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("升级代理失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhUpgradeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析升级代理响应失败: %w", err)
	}

	return &result, nil
}

// CustomUpgradeAgents 自定义升级代理
func (c *ManagerClient) CustomUpgradeAgents(ctx context.Context, req *models.WazuhCustomUpgradeRequest) (*models.WazuhUpgradeResponse, error) {
	query := url.Values{}
	for _, agentID := range req.AgentsList {
		query.Add("agents_list", agentID)
	}
	query.Set("file_path", req.FilePath)

	endpoint := "/agents/upgrade/custom?" + query.Encode()

	resp, err := c.makeRequest(ctx, "PUT", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("自定义升级代理失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhUpgradeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析自定义升级代理响应失败: %w", err)
	}

	return &result, nil
}

// GetUpgradeResult 获取升级结果
func (c *ManagerClient) GetUpgradeResult(ctx context.Context, agentsList []string) (*models.WazuhUpgradeResultResponse, error) {
	query := url.Values{}
	for _, agentID := range agentsList {
		query.Add("agents_list", agentID)
	}

	endpoint := "/agents/upgrade_result?" + query.Encode()

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取升级结果失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhUpgradeResultResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析升级结果响应失败: %w", err)
	}

	return &result, nil
}

// GetAgentStats 获取代理统计信息
func (c *ManagerClient) GetAgentStats(ctx context.Context, agentID string) (*models.WazuhAgentStatsResponse, error) {
	endpoint := fmt.Sprintf("/agents/%s/stats/agent", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取代理统计信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhAgentStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析代理统计信息响应失败: %w", err)
	}

	return &result, nil
}

// GetLogcollectorStats 获取日志收集器统计信息
func (c *ManagerClient) GetLogcollectorStats(ctx context.Context, agentID string) (*models.WazuhLogcollectorStatsResponse, error) {
	endpoint := fmt.Sprintf("/agents/%s/stats/logcollector", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取日志收集器统计信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhLogcollectorStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析日志收集器统计信息响应失败: %w", err)
	}

	return &result, nil
}

// GetAgentDaemonStats 获取代理守护进程统计信息
func (c *ManagerClient) GetAgentDaemonStats(ctx context.Context, agentID string) (*models.WazuhDaemonStatsResponse, error) {
	endpoint := fmt.Sprintf("/agents/%s/daemons/stats", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取代理守护进程统计失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhDaemonStatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析代理守护进程统计响应失败: %w", err)
	}

	return &result, nil
}

// GetCiscatResults 获取CIS-CAT扫描结果
func (c *ManagerClient) GetCiscatResults(ctx context.Context, agentID string) (*models.WazuhCiscatResponse, error) {
	endpoint := fmt.Sprintf("/ciscat/%s/results", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取CIS-CAT结果失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhCiscatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析CIS-CAT结果响应失败: %w", err)
	}

	return &result, nil
}

// GetSCAResults 获取SCA扫描结果
func (c *ManagerClient) GetSCAResults(ctx context.Context, agentID string) (*models.WazuhSCAResponse, error) {
	endpoint := fmt.Sprintf("/sca/%s", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取SCA结果失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhSCAResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析SCA结果响应失败: %w", err)
	}

	return &result, nil
}

// GetRootcheckResults 获取Rootcheck扫描结果
func (c *ManagerClient) GetRootcheckResults(ctx context.Context, agentID string, params *WazuhAgentParams) (*models.WazuhRootcheckResponse, error) {
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
	}

	endpoint := fmt.Sprintf("/rootcheck/%s", agentID)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取Rootcheck结果失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhRootcheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析Rootcheck结果响应失败: %w", err)
	}

	return &result, nil
}

// ClearRootcheckResults 清除Rootcheck扫描结果
func (c *ManagerClient) ClearRootcheckResults(ctx context.Context, agentID string) (*models.WazuhResponse, error) {
	endpoint := fmt.Sprintf("/rootcheck/%s", agentID)

	resp, err := c.makeRequest(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("清除Rootcheck结果失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析清除Rootcheck结果响应失败: %w", err)
	}

	return &result, nil
}

// GetRootcheckLastScan 获取Rootcheck最后扫描时间
func (c *ManagerClient) GetRootcheckLastScan(ctx context.Context, agentID string) (*models.WazuhRootcheckLastScanResponse, error) {
	endpoint := fmt.Sprintf("/rootcheck/%s/last_scan", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取最后扫描时间失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhRootcheckLastScanResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析最后扫描时间响应失败: %w", err)
	}

	return &result, nil
}

// RunRootcheck 运行Rootcheck扫描
func (c *ManagerClient) RunRootcheck(ctx context.Context, agentsList []string) (*models.WazuhResponse, error) {
	query := url.Values{}
	for _, agentID := range agentsList {
		query.Add("agents_list", agentID)
	}

	endpoint := "/rootcheck?" + query.Encode()

	resp, err := c.makeRequest(ctx, "PUT", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("运行Rootcheck扫描失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析Rootcheck扫描响应失败: %w", err)
	}

	return &result, nil
}

// ExecuteActiveResponse 执行主动响应
func (c *ManagerClient) ExecuteActiveResponse(ctx context.Context, req *models.WazuhActiveResponseRequest) (*models.WazuhActiveResponseResponse, error) {
	resp, err := c.makeRequest(ctx, "PUT", "/active-response", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("执行主动响应失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhActiveResponseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析主动响应响应失败: %w", err)
	}

	return &result, nil
}

// GetOverviewAgents 获取代理概览信息
func (c *ManagerClient) GetOverviewAgents(ctx context.Context) (*models.WazuhOverviewResponse, error) {
	endpoint := "/overview/agents"

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取代理概览信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析代理概览信息响应失败: %w", err)
	}

	return &result, nil
}

// GetGroupFiles 获取组文件列表
func (c *ManagerClient) GetGroupFiles(ctx context.Context, groupID string) (*models.WazuhGroupFilesResponse, error) {
	endpoint := fmt.Sprintf("/groups/%s/files", groupID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取组文件列表失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhGroupFilesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析组文件列表响应失败: %w", err)
	}

	return &result, nil
}

// GetGroupFile 获取组文件内容
func (c *ManagerClient) GetGroupFile(ctx context.Context, groupID, filename string) (*models.WazuhResponse, error) {
	endpoint := fmt.Sprintf("/groups/%s/files/%s", groupID, filename)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取组文件内容失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析组文件内容响应失败: %w", err)
	}

	return &result, nil
}

// RestartGroupAgents 重启组内代理
func (c *ManagerClient) RestartGroupAgents(ctx context.Context, groupID string) (*models.WazuhResponse, error) {
	endpoint := fmt.Sprintf("/agents/group/%s/restart", groupID)

	resp, err := c.makeRequest(ctx, "PUT", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("重启组内代理失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析重启组内代理响应失败: %w", err)
	}

	return &result, nil
}

// GetManagerLogs 获取Manager日志
func (c *ManagerClient) GetManagerLogs(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	query := url.Values{}
	if offset, ok := params["offset"].(int); ok && offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}
	if limit, ok := params["limit"].(int); ok && limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if level, ok := params["level"].(string); ok && level != "" {
		query.Set("level", level)
	}
	if search, ok := params["search"].(string); ok && search != "" {
		query.Set("search", search)
	}

	endpoint := "/manager/logs"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取Manager日志失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析Manager日志响应失败: %w", err)
	}

	return result, nil
}

// GetManagerStats 获取Manager统计信息
func (c *ManagerClient) GetManagerStats(ctx context.Context) (interface{}, error) {
	endpoint := "/manager/stats"

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取Manager统计信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析Manager统计信息响应失败: %w", err)
	}

	return result, nil
}

// RestartManager 重启Manager
func (c *ManagerClient) RestartManager(ctx context.Context) (*models.WazuhResponse, error) {
	endpoint := "/manager/restart"

	resp, err := c.makeRequest(ctx, "PUT", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("重启Manager失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析重启Manager响应失败: %w", err)
	}

	return &result, nil
}

// GetManagerConfiguration 获取Manager配置
func (c *ManagerClient) GetManagerConfiguration(ctx context.Context, section string) (interface{}, error) {
	endpoint := "/manager/configuration"
	if section != "" {
		endpoint += "/" + section
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取Manager配置失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析Manager配置响应失败: %w", err)
	}

	return result, nil
}

// UpdateManagerConfiguration 更新Manager配置
func (c *ManagerClient) UpdateManagerConfiguration(ctx context.Context, req map[string]interface{}) error {
	resp, err := c.makeRequest(ctx, "PUT", "/manager/configuration", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("更新Manager配置失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}

// UpdateAgent 更新代理信息
func (c *ManagerClient) UpdateAgent(ctx context.Context, agentID string, req *models.WazuhUpdateAgentRequest) (*models.WazuhResponse, error) {
	endpoint := fmt.Sprintf("/agents/%s", agentID)

	resp, err := c.makeRequest(ctx, "PUT", endpoint, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("更新代理失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析更新代理响应失败: %w", err)
	}

	return &result, nil
}

// GetAgentConfiguration 获取代理配置
func (c *ManagerClient) GetAgentConfiguration(ctx context.Context, agentID, section string) (interface{}, error) {
	endpoint := fmt.Sprintf("/agents/%s/config", agentID)
	if section != "" {
		endpoint += "/" + section
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取代理配置失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析代理配置响应失败: %w", err)
	}

	return result, nil
}

// UpdateAgentConfiguration 更新代理配置
func (c *ManagerClient) UpdateAgentConfiguration(ctx context.Context, agentID string, req map[string]interface{}) error {
	endpoint := fmt.Sprintf("/agents/%s/config", agentID)

	resp, err := c.makeRequest(ctx, "PUT", endpoint, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("更新代理配置失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetRules 获取规则列表
func (c *ManagerClient) GetRules(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	query := url.Values{}
	if offset, ok := params["offset"].(int); ok && offset > 0 {
		query.Set("offset", strconv.Itoa(offset))
	}
	if limit, ok := params["limit"].(int); ok && limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	if sort, ok := params["sort"].(string); ok && sort != "" {
		query.Set("sort", sort)
	}
	if search, ok := params["search"].(string); ok && search != "" {
		query.Set("search", search)
	}
	if ruleIds, ok := params["rule_ids"].(string); ok && ruleIds != "" {
		query.Set("rule_ids", ruleIds)
	}

	endpoint := "/rules"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取规则列表失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析规则列表响应失败: %w", err)
	}

	return result, nil
}

// GetAgent 获取单个代理详情
func (c *ManagerClient) GetAgent(ctx context.Context, agentID string) (*models.WazuhAgentResponse, error) {
	endpoint := fmt.Sprintf("/agents?agents_list=%s", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取代理详情失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhAgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析代理详情响应失败: %w", err)
	}

	return &result, nil
}

// AddAgent 添加新代理
func (c *ManagerClient) AddAgent(ctx context.Context, req *models.WazuhAddAgentRequest) (*models.WazuhAgentResponse, error) {
	resp, err := c.makeRequest(ctx, "POST", "/agents", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("添加代理失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhAgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析添加代理响应失败: %w", err)
	}

	return &result, nil
}

// DeleteAgent 删除代理
func (c *ManagerClient) DeleteAgent(ctx context.Context, agentID string, purge bool) (*models.WazuhResponse, error) {
	query := url.Values{}
	query.Set("agents_list", agentID)
	query.Set("status", "all")
	query.Set("older_than", "0s")

	if purge {
		query.Set("purge", "true")
	}

	endpoint := "/agents?" + query.Encode()

	resp, err := c.makeRequest(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("删除代理失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析删除代理响应失败: %w", err)
	}

	return &result, nil
}

// RestartAgent 重启代理
func (c *ManagerClient) RestartAgent(ctx context.Context, agentID string) (*models.WazuhResponse, error) {
	endpoint := fmt.Sprintf("/agents/%s/restart", agentID)

	resp, err := c.makeRequest(ctx, "PUT", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("重启代理失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析重启代理响应失败: %w", err)
	}

	return &result, nil
}

// GetAgentKey 获取代理密钥
func (c *ManagerClient) GetAgentKey(ctx context.Context, agentID string) (*models.WazuhAgentKeyResponse, error) {
	endpoint := fmt.Sprintf("/agents/%s/key", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取代理密钥失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhAgentKeyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析代理密钥响应失败: %w", err)
	}

	return &result, nil
}

// GetManagerInfo 获取Manager信息
func (c *ManagerClient) GetManagerInfo(ctx context.Context) (interface{}, error) {
	resp, err := c.makeRequest(ctx, "GET", "/manager/info", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取Manager信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhManagerInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析Manager信息响应失败: %w", err)
	}

	// 返回完整的响应结构，包含正确解析的数据
	return &result, nil
}

// GetManagerStatus 获取Manager状态
func (c *ManagerClient) GetManagerStatus(ctx context.Context) (interface{}, error) {
	resp, err := c.makeRequest(ctx, "GET", "/manager/status", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取Manager状态失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhManagerStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析Manager状态响应失败: %w", err)
	}

	// 返回完整的响应结构，包含正确解析的数据
	return &result, nil
}

// HealthCheck 健康检查
func (c *ManagerClient) HealthCheck(ctx context.Context) error {
	resp, err := c.makeRequest(ctx, "GET", "/", nil)
	if err != nil {
		return fmt.Errorf("Wazuh Manager健康检查失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Wazuh Manager健康检查失败，状态码: %d", resp.StatusCode)
	}

	return nil
}

// WazuhAgentParams 代理查询参数
type WazuhAgentParams struct {
	Offset  int    `json:"offset,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Sort    string `json:"sort,omitempty"`
	Search  string `json:"search,omitempty"`
	Status  string `json:"status,omitempty"`
	OS      string `json:"os,omitempty"`
	Version string `json:"version,omitempty"`
	Group   string `json:"group,omitempty"`
}

// WazuhNetworkParams 网络查询参数
type WazuhNetworkParams struct {
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Sort   string `json:"sort,omitempty"`
	Search string `json:"search,omitempty"`
}

// WazuhProcessParams 进程查询参数
type WazuhProcessParams struct {
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Sort   string `json:"sort,omitempty"`
	Search string `json:"search,omitempty"`
}

// WazuhPackageParams 软件包查询参数
type WazuhPackageParams struct {
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Sort   string `json:"sort,omitempty"`
	Search string `json:"search,omitempty"`
}

// WazuhPortParams 端口查询参数
type WazuhPortParams struct {
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Sort   string `json:"sort,omitempty"`
	Search string `json:"search,omitempty"`
}

// WazuhHotfixParams 热修复查询参数
type WazuhHotfixParams struct {
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Sort   string `json:"sort,omitempty"`
	Search string `json:"search,omitempty"`
}

// GetAgentsSummary 获取代理状态汇总
func (c *ManagerClient) GetAgentsSummary(ctx context.Context) (*models.WazuhAgentsSummaryResponse, error) {
	endpoint := "/agents/summary/status"

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取代理状态汇总失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhAgentsSummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析代理状态汇总响应失败: %w", err)
	}

	return &result, nil
}

// GetGroups 获取组列表
func (c *ManagerClient) GetGroups(ctx context.Context, params *WazuhGroupParams) (*models.WazuhGroupListResponse, error) {
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
	}

	endpoint := "/groups"
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取组列表失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhGroupListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析组列表响应失败: %w", err)
	}

	return &result, nil
}

// CreateGroup 创建代理组
func (c *ManagerClient) CreateGroup(ctx context.Context, groupID string) (*models.WazuhGroupResponse, error) {
	req := map[string]string{"group_id": groupID}

	resp, err := c.makeRequest(ctx, "POST", "/groups", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("创建代理组失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhGroupResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析创建代理组响应失败: %w", err)
	}

	return &result, nil
}

// GetGroupAgents 获取组内代理
func (c *ManagerClient) GetGroupAgents(ctx context.Context, groupID string, params *WazuhAgentParams) (*models.WazuhGroupAgentsResponse, error) {
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
	}

	endpoint := fmt.Sprintf("/groups/%s/agents", groupID)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取组内代理失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhGroupAgentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析组内代理响应失败: %w", err)
	}

	return &result, nil
}

// AssignAgentToGroup 将代理分配到组
func (c *ManagerClient) AssignAgentToGroup(ctx context.Context, agentID, groupID string) (*models.WazuhResponse, error) {
	query := url.Values{}
	query.Set("agents_list", agentID)
	query.Set("group_id", groupID)

	endpoint := "/agents/group?" + query.Encode()

	resp, err := c.makeRequest(ctx, "PUT", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("分配代理到组失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析分配代理到组响应失败: %w", err)
	}

	return &result, nil
}

// GetGroupConfiguration 获取组配置
func (c *ManagerClient) GetGroupConfiguration(ctx context.Context, groupID string) (*models.WazuhGroupConfigResponse, error) {
	endpoint := fmt.Sprintf("/groups/%s/configuration", groupID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取组配置失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhGroupConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析组配置响应失败: %w", err)
	}

	return &result, nil
}

// GetSystemInfo 获取系统信息
func (c *ManagerClient) GetSystemInfo(ctx context.Context, agentID string) (*models.WazuhSystemInfoResponse, error) {
	endpoint := fmt.Sprintf("/syscollector/%s/os", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取系统信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhSystemInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析系统信息响应失败: %w", err)
	}

	return &result, nil
}

// GetHardwareInfo 获取硬件信息
func (c *ManagerClient) GetHardwareInfo(ctx context.Context, agentID string) (*models.WazuhHardwareInfoResponse, error) {
	endpoint := fmt.Sprintf("/syscollector/%s/hardware", agentID)

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取硬件信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhHardwareInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析硬件信息响应失败: %w", err)
	}

	return &result, nil
}

// GetNetworkAddresses 获取网络地址信息
func (c *ManagerClient) GetNetworkAddresses(ctx context.Context, agentID string, params *WazuhNetworkParams) (*models.WazuhNetworkAddressListResponse, error) {
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
	}

	endpoint := fmt.Sprintf("/syscollector/%s/netaddr", agentID)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取网络地址信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhNetworkAddressListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析网络地址信息响应失败: %w", err)
	}

	return &result, nil
}

// GetProcesses 获取进程信息
func (c *ManagerClient) GetProcesses(ctx context.Context, agentID string, params *WazuhProcessParams) (*models.WazuhProcessListResponse, error) {
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
	}

	endpoint := fmt.Sprintf("/syscollector/%s/processes", agentID)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取进程信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhProcessListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析进程信息响应失败: %w", err)
	}

	return &result, nil
}

// GetPackages 获取软件包信息
func (c *ManagerClient) GetPackages(ctx context.Context, agentID string, params *WazuhPackageParams) (*models.WazuhPackageListResponse, error) {
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
	}

	endpoint := fmt.Sprintf("/syscollector/%s/packages", agentID)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取软件包信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhPackageListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析软件包信息响应失败: %w", err)
	}

	return &result, nil
}

// GetPorts 获取端口信息
func (c *ManagerClient) GetPorts(ctx context.Context, agentID string, params *WazuhPortParams) (*models.WazuhPortListResponse, error) {
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
	}

	endpoint := fmt.Sprintf("/syscollector/%s/ports", agentID)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取端口信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhPortListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析端口信息响应失败: %w", err)
	}

	return &result, nil
}

// GetHotfixes 获取热修复信息
func (c *ManagerClient) GetHotfixes(ctx context.Context, agentID string, params *WazuhHotfixParams) (*models.WazuhHotfixListResponse, error) {
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
	}

	endpoint := fmt.Sprintf("/syscollector/%s/hotfixes", agentID)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取热修复信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhHotfixListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析热修复信息响应失败: %w", err)
	}

	return &result, nil
}

// GetNetworkProtocols 获取网络协议信息
func (c *ManagerClient) GetNetworkProtocols(ctx context.Context, agentID string, params *WazuhNetworkParams) (*models.WazuhNetworkProtocolListResponse, error) {
	query := url.Values{}
	if params != nil {
		if params.Offset > 0 {
			query.Set("offset", strconv.Itoa(params.Offset))
		}
		if params.Limit > 0 {
			query.Set("limit", strconv.Itoa(params.Limit))
		}
		if params.Sort != "" {
			query.Set("sort", params.Sort)
		}
		if params.Search != "" {
			query.Set("search", params.Search)
		}
	}

	endpoint := fmt.Sprintf("/syscollector/%s/netproto", agentID)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.makeRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("获取网络协议信息失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result models.WazuhNetworkProtocolListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析网络协议信息响应失败: %w", err)
	}

	return &result, nil
}
