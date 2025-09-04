package models

import (
	"encoding/json"
	"strings"
	"time"
)

// NullableTime 可空时间类型
type NullableTime struct {
	*time.Time
}

// UnmarshalJSON 自定义JSON反序列化，处理空字符串和无效时间格式
func (nt *NullableTime) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	// 处理空字符串或只包含空格的字符串
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		nt.Time = nil
		return nil
	}

	// 尝试解析时间
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		formats := []string{
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02",
		}

		for _, format := range formats {
			if t, err = time.Parse(format, s); err == nil {
				break
			}
		}

		if err != nil {
			nt.Time = nil
			return nil
		}
	}

	nt.Time = &t
	return nil
}

// MarshalJSON 自定义JSON序列化
func (nt NullableTime) MarshalJSON() ([]byte, error) {
	if nt.Time == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(nt.Time.Format(time.RFC3339))
}

// WazuhAgent Wazuh代理信息
type WazuhAgent struct {
	ID                string       `json:"id"`
	Name              string       `json:"name"`
	IP                string       `json:"ip"`
	RegisterIP        string       `json:"register_ip"`
	Status            string       `json:"status"`
	StatusCode        int          `json:"status_code"`
	OS                *WazuhOSInfo `json:"os,omitempty"`
	Version           string       `json:"version"`
	Manager           string       `json:"manager"`
	NodeName          string       `json:"node_name"`
	DateAdd           time.Time    `json:"date_add"`
	LastKeepAlive     *time.Time   `json:"last_keepalive,omitempty"`
	DisconnectionTime *time.Time   `json:"disconnection_time,omitempty"`
	Group             []string     `json:"group"`
	ConfigSum         string       `json:"config_sum"`
	MergedSum         string       `json:"merged_sum"`
	GroupConfigStatus string       `json:"group_config_status"`
}

// WazuhOSInfo 操作系统信息
type WazuhOSInfo struct {
	Build          string `json:"build,omitempty"`
	Major          string `json:"major,omitempty"`
	Minor          string `json:"minor,omitempty"`
	Name           string `json:"name"`
	Platform       string `json:"platform"`
	UName          string `json:"uname,omitempty"`
	Version        string `json:"version"`
	DisplayVersion string `json:"display_version,omitempty"`
}

// WazuhHardwareInfo 硬件信息
type WazuhHardwareInfo struct {
	AgentID     string        `json:"agent_id"`
	CPU         *WazuhCPUInfo `json:"cpu,omitempty"`
	RAM         *WazuhRAMInfo `json:"ram,omitempty"`
	BoardSerial string        `json:"board_serial,omitempty"`
	ScanID      int           `json:"scan_id,omitempty"`
	ScanTime    *time.Time    `json:"scan_time,omitempty"`
}

// WazuhCPUInfo CPU信息
type WazuhCPUInfo struct {
	Name  string `json:"name"`
	Cores int    `json:"cores"`
	MHz   int    `json:"mhz"`
}

// WazuhRAMInfo 内存信息
type WazuhRAMInfo struct {
	Total int `json:"total"`
	Free  int `json:"free"`
	Usage int `json:"usage"`
}

// WazuhProcessInfo 进程信息
type WazuhProcessInfo struct {
	AgentID   string `json:"agent_id"`
	PID       string `json:"pid"`
	Name      string `json:"name"`
	CMD       string `json:"cmd"`
	PPID      int    `json:"ppid"`
	UTime     int    `json:"utime"`
	STime     int    `json:"stime"`
	StartTime int64  `json:"start_time"`
	VMSize    int64  `json:"vm_size"`
	Size      int64  `json:"size"`
	Priority  int    `json:"priority"`
	NLWP      int    `json:"nlwp"`
	Session   int    `json:"session"`
	ScanID    int    `json:"scan_id,omitempty"`
	ScanTime  string `json:"scan_time,omitempty"`
}

// WazuhPortInfo 端口信息
type WazuhPortInfo struct {
	AgentID  string                `json:"agent_id"`
	Local    *WazuhNetworkEndpoint `json:"local"`
	Remote   *WazuhNetworkEndpoint `json:"remote"`
	PID      int                   `json:"pid"`
	Protocol string                `json:"protocol"`
	RXQueue  int                   `json:"rx_queue"`
	TXQueue  int                   `json:"tx_queue"`
	Inode    int                   `json:"inode"`
	State    string                `json:"state,omitempty"`
	Process  string                `json:"process,omitempty"`
	ScanID   int                   `json:"scan_id,omitempty"`
	ScanTime string                `json:"scan_time,omitempty"`
}

// WazuhNetworkEndpoint 网络端点
type WazuhNetworkEndpoint struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// WazuhPackageInfo 软件包信息
type WazuhPackageInfo struct {
	AgentID      string       `json:"agent_id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Vendor       string       `json:"vendor,omitempty"`
	Description  string       `json:"description,omitempty"`
	Size         int64        `json:"size"`
	Architecture string       `json:"architecture,omitempty"`
	Format       string       `json:"format"`
	Location     string       `json:"location,omitempty"`
	Priority     string       `json:"priority,omitempty"`
	Section      string       `json:"section,omitempty"`
	Source       string       `json:"source,omitempty"`
	InstallTime  NullableTime `json:"install_time,omitempty"`
	ScanID       int          `json:"scan_id,omitempty"`
	ScanTime     string       `json:"scan_time,omitempty"`
}

// WazuhSearchQuery 事件搜索查询
type WazuhSearchQuery struct {
	Index     string                   `json:"index,omitempty"`      // 索引名称
	IndexType string                   `json:"index_type,omitempty"` // alerts or archives
	Query     interface{}              `json:"query,omitempty"`      // 支持字符串或复杂DSL
	Size      int                      `json:"size,omitempty"`
	From      int                      `json:"from,omitempty"`
	Sort      string                   `json:"sort,omitempty"`       // 简化为字符串
	Fields    []string                 `json:"fields,omitempty"`     // 返回字段
	StartTime time.Time                `json:"start_time,omitempty"` // 开始时间
	EndTime   time.Time                `json:"end_time,omitempty"`   // 结束时间
	AgentID   string                   `json:"agent_id,omitempty"`   // 代理ID
	RuleID    string                   `json:"rule_id,omitempty"`    // 规则ID
	RuleLevel int                      `json:"rule_level,omitempty"` // 规则级别
}

// WazuhSearchResponse 搜索响应
type WazuhSearchResponse struct {
	Took     int                    `json:"took"`
	TimedOut bool                   `json:"timed_out"`
	Shards   map[string]interface{} `json:"_shards"`
	Hits     *WazuhSearchHits       `json:"hits"`
}

// WazuhSearchHits 搜索结果
type WazuhSearchHits struct {
	Total    map[string]interface{} `json:"total"`
	MaxScore float64                `json:"max_score"`
	Hits     []*WazuhSearchHit      `json:"hits"`
}

// WazuhSearchHit 单个搜索结果
type WazuhSearchHit struct {
	Index  string                 `json:"_index"`
	Type   string                 `json:"_type"`
	ID     string                 `json:"_id"`
	Score  float64                `json:"_score"`
	Source map[string]interface{} `json:"_source"`
}

// WazuhAgentListResponse 代理列表响应
type WazuhAgentListResponse struct {
	Data    WazuhAgentListData `json:"data"`
	Message string             `json:"message"`
	Error   int                `json:"error"`
}

// WazuhAgentListData 代理列表数据
type WazuhAgentListData struct {
	AffectedItems      []*WazuhAgent `json:"affected_items"`
	TotalAffectedItems int           `json:"total_affected_items"`
	TotalFailedItems   int           `json:"total_failed_items"`
	FailedItems        []interface{} `json:"failed_items"`
}

// WazuhAgentResponse 单个代理响应
type WazuhAgentResponse struct {
	Data    WazuhAgentData `json:"data"`
	Message string         `json:"message"`
	Error   int            `json:"error"`
}

// WazuhAgentData 代理数据
type WazuhAgentData struct {
	AffectedItems      []*WazuhAgent `json:"affected_items"`
	TotalAffectedItems int           `json:"total_affected_items"`
	TotalFailedItems   int           `json:"total_failed_items"`
	FailedItems        []interface{} `json:"failed_items"`
}

// WazuhAddAgentRequest 添加代理请求
type WazuhAddAgentRequest struct {
	Name string `json:"name" binding:"required"`
	IP   string `json:"ip,omitempty"`
}

// WazuhUpdateAgentRequest 更新代理请求
type WazuhUpdateAgentRequest struct {
	Name string `json:"name,omitempty"`
	IP   string `json:"ip,omitempty"`
}

// WazuhUpgradeRequest 代理升级请求
type WazuhUpgradeRequest struct {
	AgentsList     []string `json:"agents_list" binding:"required"`
	WPKRepo        string   `json:"wpk_repo,omitempty"`
	UpgradeVersion string   `json:"upgrade_version,omitempty"`
	Force          bool     `json:"force,omitempty"`
	PackageType    string   `json:"package_type,omitempty"`
	OSPlatform     string   `json:"os_platform,omitempty"`
	OSVersion      string   `json:"os_version,omitempty"`
	OSName         string   `json:"os_name,omitempty"`
	Version        string   `json:"version,omitempty"`
	Group          string   `json:"group,omitempty"`
	NodeName       string   `json:"node_name,omitempty"`
	Name           string   `json:"name,omitempty"`
	IP             string   `json:"ip,omitempty"`
	RegisterIP     string   `json:"register_ip,omitempty"`
}

// WazuhUpgradeResponse 升级响应
type WazuhUpgradeResponse struct {
	Data    WazuhUpgradeData `json:"data"`
	Message string           `json:"message"`
	Error   int              `json:"error"`
}

// WazuhUpgradeData 升级数据
type WazuhUpgradeData struct {
	AffectedItems      []WazuhUpgradeTask `json:"affected_items"`
	TotalAffectedItems int                `json:"total_affected_items"`
	TotalFailedItems   int                `json:"total_failed_items"`
	FailedItems        []interface{}      `json:"failed_items"`
}

// WazuhUpgradeTask 升级任务
type WazuhUpgradeTask struct {
	Agent  string `json:"agent"`
	TaskID int    `json:"task_id"`
}

// WazuhCustomUpgradeRequest 自定义升级请求
type WazuhCustomUpgradeRequest struct {
	AgentsList []string `json:"agents_list" binding:"required"`
	FilePath   string   `json:"file_path" binding:"required"`
}

// WazuhUpgradeResultResponse 升级结果响应
type WazuhUpgradeResultResponse struct {
	Data    WazuhUpgradeResultData `json:"data"`
	Message string                 `json:"message"`
	Error   int                    `json:"error"`
}

// WazuhUpgradeResultData 升级结果数据
type WazuhUpgradeResultData struct {
	AffectedItems      []WazuhUpgradeResult `json:"affected_items"`
	TotalAffectedItems int                  `json:"total_affected_items"`
	TotalFailedItems   int                  `json:"total_failed_items"`
	FailedItems        []interface{}        `json:"failed_items"`
}

// WazuhUpgradeResult 升级结果
type WazuhUpgradeResult struct {
	Agent      string    `json:"agent"`
	TaskID     int       `json:"task_id"`
	Node       string    `json:"node"`
	Module     string    `json:"module"`
	Command    string    `json:"command"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	CreateTime time.Time `json:"create_time"`
	UpdateTime time.Time `json:"update_time"`
}

// WazuhSystemInfoResponse 系统信息响应
type WazuhSystemInfoResponse struct {
	Data    WazuhSystemInfoData `json:"data"`
	Message string              `json:"message"`
	Error   int                 `json:"error"`
}

// WazuhSystemInfoData 系统信息数据
type WazuhSystemInfoData struct {
	AffectedItems      []WazuhSystemInfo `json:"affected_items"`
	TotalAffectedItems int               `json:"total_affected_items"`
	TotalFailedItems   int               `json:"total_failed_items"`
	FailedItems        []interface{}     `json:"failed_items"`
}

// WazuhSystemInfo 系统信息
type WazuhSystemInfo struct {
	AgentID  string `json:"agent_id"`
	ScanID   int    `json:"scan_id,omitempty"`
	ScanTime string `json:"scan_time,omitempty"`
}

// WazuhHardwareInfoResponse 硬件信息响应
type WazuhHardwareInfoResponse struct {
	Data    WazuhHardwareInfoData `json:"data"`
	Message string                `json:"message"`
	Error   int                   `json:"error"`
}

// WazuhHardwareInfoData 硬件信息数据
type WazuhHardwareInfoData struct {
	AffectedItems      []WazuhHardwareInfo `json:"affected_items"`
	TotalAffectedItems int                 `json:"total_affected_items"`
	TotalFailedItems   int                 `json:"total_failed_items"`
	FailedItems        []interface{}       `json:"failed_items"`
}

// WazuhNetworkAddressInfo 网络地址信息
type WazuhNetworkAddressInfo struct {
	AgentID   string `json:"agent_id"`
	Interface string `json:"iface"`
	Address   string `json:"address"`
	Netmask   string `json:"netmask"`
	Protocol  string `json:"proto"`
	Broadcast string `json:"broadcast"`
	ScanID    int    `json:"scan_id,omitempty"`
	ScanTime  string `json:"scan_time,omitempty"`
}

// WazuhNetworkAddressListResponse 网络地址列表响应
type WazuhNetworkAddressListResponse struct {
	Data    WazuhNetworkAddressListData `json:"data"`
	Message string                      `json:"message"`
	Error   int                         `json:"error"`
}

// WazuhNetworkAddressListData 网络地址列表数据
type WazuhNetworkAddressListData struct {
	AffectedItems      []WazuhNetworkAddressInfo `json:"affected_items"`
	TotalAffectedItems int                       `json:"total_affected_items"`
	TotalFailedItems   int                       `json:"total_failed_items"`
	FailedItems        []interface{}             `json:"failed_items"`
}

// WazuhProcessListResponse 进程列表响应
type WazuhProcessListResponse struct {
	Data    WazuhProcessListData `json:"data"`
	Message string               `json:"message"`
	Error   int                  `json:"error"`
}

// WazuhProcessListData 进程列表数据
type WazuhProcessListData struct {
	AffectedItems      []WazuhProcessInfo `json:"affected_items"`
	TotalAffectedItems int                `json:"total_affected_items"`
	TotalFailedItems   int                `json:"total_failed_items"`
	FailedItems        []interface{}      `json:"failed_items"`
}

// WazuhPackageListResponse 软件包列表响应
type WazuhPackageListResponse struct {
	Data    WazuhPackageListData `json:"data"`
	Message string               `json:"message"`
	Error   int                  `json:"error"`
}

// WazuhPackageListData 软件包列表数据
type WazuhPackageListData struct {
	AffectedItems      []WazuhPackageInfo `json:"affected_items"`
	TotalAffectedItems int                `json:"total_affected_items"`
	TotalFailedItems   int                `json:"total_failed_items"`
	FailedItems        []interface{}      `json:"failed_items"`
}

// WazuhPortListResponse 端口列表响应
type WazuhPortListResponse struct {
	Data    WazuhPortListData `json:"data"`
	Message string            `json:"message"`
	Error   int               `json:"error"`
}

// WazuhPortListData 端口列表数据
type WazuhPortListData struct {
	AffectedItems      []WazuhPortInfo `json:"affected_items"`
	TotalAffectedItems int             `json:"total_affected_items"`
	TotalFailedItems   int             `json:"total_failed_items"`
	FailedItems        []interface{}   `json:"failed_items"`
}

// WazuhHotfixInfo 热修复信息
type WazuhHotfixInfo struct {
	AgentID  string `json:"agent_id"`
	Hotfix   string `json:"hotfix"`
	ScanID   int    `json:"scan_id,omitempty"`
	ScanTime string `json:"scan_time,omitempty"`
}

// WazuhHotfixListResponse 热修复列表响应
type WazuhHotfixListResponse struct {
	Data    WazuhHotfixListData `json:"data"`
	Message string              `json:"message"`
	Error   int                 `json:"error"`
}

// WazuhHotfixListData 热修复列表数据
type WazuhHotfixListData struct {
	AffectedItems      []WazuhHotfixInfo `json:"affected_items"`
	TotalAffectedItems int               `json:"total_affected_items"`
	TotalFailedItems   int               `json:"total_failed_items"`
	FailedItems        []interface{}     `json:"failed_items"`
}

// WazuhNetworkProtocolInfo 网络协议信息
type WazuhNetworkProtocolInfo struct {
	AgentID   string `json:"agent_id"`
	Interface string `json:"iface"`
	Type      string `json:"type"`
	Gateway   string `json:"gateway"`
	DHCP      string `json:"dhcp"`
	ScanID    int    `json:"scan_id,omitempty"`
	ScanTime  string `json:"scan_time,omitempty"`
}

// WazuhNetworkProtocolListResponse 网络协议列表响应
type WazuhNetworkProtocolListResponse struct {
	Data    WazuhNetworkProtocolListData `json:"data"`
	Message string                       `json:"message"`
	Error   int                          `json:"error"`
}

// WazuhNetworkProtocolListData 网络协议列表数据
type WazuhNetworkProtocolListData struct {
	AffectedItems      []WazuhNetworkProtocolInfo `json:"affected_items"`
	TotalAffectedItems int                        `json:"total_affected_items"`
	TotalFailedItems   int                        `json:"total_failed_items"`
	FailedItems        []interface{}              `json:"failed_items"`
}

// WazuhAgentStatsResponse 代理统计响应
type WazuhAgentStatsResponse struct {
	Data    WazuhAgentStatsData `json:"data"`
	Message string              `json:"message"`
	Error   int                 `json:"error"`
}

// WazuhAgentStatsData 代理统计数据
type WazuhAgentStatsData struct {
	AffectedItems      []WazuhAgentStats `json:"affected_items"`
	TotalAffectedItems int               `json:"total_affected_items"`
	TotalFailedItems   int               `json:"total_failed_items"`
	FailedItems        []interface{}     `json:"failed_items"`
}

// WazuhAgentStats 代理统计信息
type WazuhAgentStats struct {
	Status        string     `json:"status"`
	LastKeepAlive *time.Time `json:"last_keepalive,omitempty"`
	LastAck       *time.Time `json:"last_ack,omitempty"`
	MsgCount      int        `json:"msg_count"`
	MsgSent       int        `json:"msg_sent"`
	MsgBuffer     int        `json:"msg_buffer"`
	BufferEnabled bool       `json:"buffer_enabled"`
}

// WazuhLogcollectorStatsResponse 日志收集器统计响应
type WazuhLogcollectorStatsResponse struct {
	Data    WazuhLogcollectorStatsResponseData `json:"data"`
	Message string                             `json:"message"`
	Error   int                                `json:"error"`
}

// WazuhLogcollectorStatsResponseData 日志收集器统计响应数据
type WazuhLogcollectorStatsResponseData struct {
	AffectedItems      []WazuhLogcollectorStats `json:"affected_items"`
	TotalAffectedItems int                      `json:"total_affected_items"`
	TotalFailedItems   int                      `json:"total_failed_items"`
	FailedItems        []interface{}            `json:"failed_items"`
}

// WazuhLogcollectorStats 日志收集器统计信息
type WazuhLogcollectorStats struct {
	Global   WazuhLogcollectorStatsData `json:"global"`
	Interval WazuhLogcollectorStatsData `json:"interval"`
}

// WazuhLogcollectorStatsData 日志收集器统计数据
type WazuhLogcollectorStatsData struct {
	Start time.Time                    `json:"start"`
	End   time.Time                    `json:"end"`
	Files []WazuhLogcollectorFileStats `json:"files"`
}

// WazuhLogcollectorFileStats 日志收集器文件统计
type WazuhLogcollectorFileStats struct {
	Location string                         `json:"location"`
	Events   int                            `json:"events"`
	Bytes    int                            `json:"bytes"`
	Targets  []WazuhLogcollectorTargetStats `json:"targets"`
}

// WazuhLogcollectorTargetStats 日志收集器目标统计
type WazuhLogcollectorTargetStats struct {
	Name  string `json:"name"`
	Drops int    `json:"drops"`
}

// WazuhDaemonStatsResponse 守护进程统计响应
type WazuhDaemonStatsResponse struct {
	Data    WazuhDaemonStatsData `json:"data"`
	Message string               `json:"message"`
	Error   int                  `json:"error"`
}

// WazuhDaemonStatsData 守护进程统计数据
type WazuhDaemonStatsData struct {
	AffectedItems      []WazuhDaemonStats `json:"affected_items"`
	TotalAffectedItems int                `json:"total_affected_items"`
	TotalFailedItems   int                `json:"total_failed_items"`
	FailedItems        []interface{}      `json:"failed_items"`
}

// WazuhDaemonStats 守护进程统计信息
type WazuhDaemonStats struct {
	Timestamp string                 `json:"timestamp"`
	Name      string                 `json:"name"`
	Agents    []WazuhDaemonAgentStat `json:"agents"`
}

// WazuhDaemonAgentStat 守护进程代理统计
type WazuhDaemonAgentStat struct {
	ID      int                    `json:"id"`
	Uptime  string                 `json:"uptime"`
	Metrics map[string]interface{} `json:"metrics"`
}

// WazuhCiscatResponse CIS-CAT响应
type WazuhCiscatResponse struct {
	Data    WazuhCiscatData `json:"data"`
	Message string          `json:"message"`
	Error   int             `json:"error"`
}

// WazuhCiscatData CIS-CAT数据
type WazuhCiscatData struct {
	AffectedItems      []WazuhCiscatResult `json:"affected_items"`
	TotalAffectedItems int                 `json:"total_affected_items"`
	TotalFailedItems   int                 `json:"total_failed_items"`
	FailedItems        []interface{}       `json:"failed_items"`
}

// WazuhCiscatResult CIS-CAT结果
type WazuhCiscatResult struct {
	Benchmark  string              `json:"benchmark"`
	Profile    string              `json:"profile"`
	Pass       int                 `json:"pass"`
	Fail       int                 `json:"fail"`
	Error      int                 `json:"error"`
	NotChecked int                 `json:"notchecked"`
	Unknown    int                 `json:"unknown"`
	Score      int                 `json:"score"`
	Scan       WazuhCiscatScanInfo `json:"scan"`
}

// WazuhCiscatScanInfo CIS-CAT扫描信息
type WazuhCiscatScanInfo struct {
	ID   int    `json:"id"`
	Time string `json:"time"`
}

// WazuhSCAResponse SCA响应
type WazuhSCAResponse struct {
	Data    WazuhSCAData `json:"data"`
	Message string       `json:"message"`
	Error   int          `json:"error"`
}

// WazuhSCAData SCA数据
type WazuhSCAData struct {
	AffectedItems      []WazuhSCAResult `json:"affected_items"`
	TotalAffectedItems int              `json:"total_affected_items"`
	TotalFailedItems   int              `json:"total_failed_items"`
	FailedItems        []interface{}    `json:"failed_items"`
}

// WazuhSCAResult SCA扫描结果
type WazuhSCAResult struct {
	PolicyID    string    `json:"policy_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	References  string    `json:"references"`
	TotalChecks int       `json:"total_checks"`
	Pass        int       `json:"pass"`
	Fail        int       `json:"fail"`
	Invalid     int       `json:"invalid"`
	Score       int       `json:"score"`
	StartScan   string    `json:"start_scan"`
	EndScan     string    `json:"end_scan"`
	HashFile    string    `json:"hash_file"`
}

// WazuhRootcheckResponse Rootcheck响应
type WazuhRootcheckResponse struct {
	Data    WazuhRootcheckData `json:"data"`
	Message string             `json:"message"`
	Error   int                `json:"error"`
}

// WazuhRootcheckData Rootcheck数据
type WazuhRootcheckData struct {
	AffectedItems      []WazuhRootcheckResult `json:"affected_items"`
	TotalAffectedItems int                    `json:"total_affected_items"`
	TotalFailedItems   int                    `json:"total_failed_items"`
	FailedItems        []interface{}          `json:"failed_items"`
}

// WazuhRootcheckResult Rootcheck结果
type WazuhRootcheckResult struct {
	DateFirst string `json:"date_first"`
	DateLast  string `json:"date_last"`
	Log       string `json:"log"`
	Status    string `json:"status"`
}

// WazuhRootcheckLastScanResponse 最后扫描响应
type WazuhRootcheckLastScanResponse struct {
	Data    WazuhRootcheckLastScanData `json:"data"`
	Message string                     `json:"message"`
	Error   int                        `json:"error"`
}

// WazuhRootcheckLastScanData 最后扫描数据
type WazuhRootcheckLastScanData struct {
	AffectedItems      []WazuhRootcheckLastScan `json:"affected_items"`
	TotalAffectedItems int                      `json:"total_affected_items"`
	TotalFailedItems   int                      `json:"total_failed_items"`
	FailedItems        []interface{}            `json:"failed_items"`
}

// WazuhRootcheckLastScan 最后扫描
type WazuhRootcheckLastScan struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// WazuhActiveResponseRequest 主动响应请求
type WazuhActiveResponseRequest struct {
	Command   string                 `json:"command" binding:"required"`
	Arguments []string               `json:"arguments,omitempty"`
	Alert     map[string]interface{} `json:"alert,omitempty"`
}

// WazuhActiveResponseResponse Active Response响应
type WazuhActiveResponseResponse struct {
	Data    WazuhActiveResponseData `json:"data"`
	Message string                  `json:"message"`
	Error   int                     `json:"error"`
}

// WazuhActiveResponseData Active Response数据
type WazuhActiveResponseData struct {
	AffectedItems      []string      `json:"affected_items"`
	TotalAffectedItems int           `json:"total_affected_items"`
	TotalFailedItems   int           `json:"total_failed_items"`
	FailedItems        []interface{} `json:"failed_items"`
}

// WazuhOverviewResponse 概览响应
type WazuhOverviewResponse struct {
	Data    WazuhOverviewData `json:"data"`
	Message string            `json:"message,omitempty"`
	Error   int               `json:"error"`
}

// WazuhOverviewData 概览数据
type WazuhOverviewData struct {
	Nodes               []WazuhOverviewNode         `json:"nodes"`
	Groups              []WazuhOverviewGroup        `json:"groups"`
	AgentOS             []WazuhOverviewAgentOS      `json:"agent_os"`
	AgentStatus         WazuhAgentsSummary          `json:"agent_status"`
	AgentVersion        []WazuhOverviewAgentVersion `json:"agent_version"`
	LastRegisteredAgent []WazuhOverviewLastAgent    `json:"last_registered_agent"`
}

// WazuhOverviewNode 概览节点
type WazuhOverviewNode struct {
	NodeName string `json:"node_name"`
	Count    int    `json:"count"`
}

// WazuhOverviewGroup 概览组
type WazuhOverviewGroup struct {
	Name      string `json:"name"`
	Count     int    `json:"count"`
	MergedSum string `json:"mergedSum"`
	ConfigSum string `json:"configSum"`
}

// WazuhOverviewAgentOS 概览代理操作系统
type WazuhOverviewAgentOS struct {
	OS    WazuhOSInfo `json:"os"`
	Count int         `json:"count"`
}

// WazuhOverviewAgentVersion 概览代理版本
type WazuhOverviewAgentVersion struct {
	Version string `json:"version"`
	Count   int    `json:"count"`
}

// WazuhOverviewLastAgent 概览最后注册代理
type WazuhOverviewLastAgent struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	IP                string `json:"ip"`
	RegisterIP        string `json:"registerIP"`
	Status            string `json:"status"`
	StatusCode        int    `json:"status_code"`
	NodeName          string `json:"node_name"`
	DateAdd           string `json:"dateAdd"`
	GroupConfigStatus string `json:"group_config_status"`
}

// WazuhGroupFilesResponse 组文件响应
type WazuhGroupFilesResponse struct {
	Data    WazuhGroupFilesData `json:"data"`
	Message string              `json:"message"`
	Error   int                 `json:"error"`
}

// WazuhGroupFilesData 组文件数据
type WazuhGroupFilesData struct {
	AffectedItems      []WazuhGroupFile `json:"affected_items"`
	TotalAffectedItems int              `json:"total_affected_items"`
	TotalFailedItems   int              `json:"total_failed_items"`
	FailedItems        []interface{}    `json:"failed_items"`
}

// WazuhGroupFile 组文件
type WazuhGroupFile struct {
	Filename string `json:"filename"`
	Hash     string `json:"hash"`
}

// WazuhAgentKeyResponse 代理密钥响应
type WazuhAgentKeyResponse struct {
	Data    WazuhAgentKeyData `json:"data"`
	Message string            `json:"message"`
	Error   int               `json:"error"`
}

// WazuhAgentKeyData 代理密钥数据
type WazuhAgentKeyData struct {
	AffectedItems      []WazuhAgentKey `json:"affected_items"`
	TotalAffectedItems int             `json:"total_affected_items"`
	TotalFailedItems   int             `json:"total_failed_items"`
	FailedItems        []interface{}   `json:"failed_items"`
}

// WazuhAgentKey 代理密钥
type WazuhAgentKey struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// WazuhDynamicAuthRequest 动态认证配置请求
type WazuhDynamicAuthRequest struct {
	Manager *WazuhManagerAuthConfig `json:"manager,omitempty"`
	Indexer *WazuhIndexerAuthConfig `json:"indexer,omitempty"`
}

// WazuhManagerAuthConfig Manager认证配置
type WazuhManagerAuthConfig struct {
	URL       string `json:"url" binding:"required"`
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	Timeout   string `json:"timeout,omitempty"`
	TLSVerify *bool  `json:"tls_verify,omitempty"`
}

// WazuhIndexerAuthConfig Indexer认证配置
type WazuhIndexerAuthConfig struct {
	URL       string `json:"url" binding:"required"`
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	Timeout   string `json:"timeout,omitempty"`
	TLSVerify *bool  `json:"tls_verify,omitempty"`
}

// WazuhConfigResponse 配置响应（脱敏）
type WazuhConfigResponse struct {
	Manager *WazuhManagerConfigInfo `json:"manager,omitempty"`
	Indexer *WazuhIndexerConfigInfo `json:"indexer,omitempty"`
	Status  string                  `json:"status"`
	Message string                  `json:"message,omitempty"`
}

// WazuhManagerConfigInfo Manager配置信息（脱敏）
type WazuhManagerConfigInfo struct {
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password"` // 脱敏显示
	Timeout   string `json:"timeout"`
	TLSVerify bool   `json:"tls_verify"`
	Status    string `json:"status"`
}

// WazuhIndexerConfigInfo Indexer配置信息（脱敏）
type WazuhIndexerConfigInfo struct {
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password"` // 脱敏显示
	Timeout   string `json:"timeout"`
	TLSVerify bool   `json:"tls_verify"`
	Status    string `json:"status"`
}

// WazuhAuthTestRequest 认证测试请求
type WazuhAuthTestRequest struct {
	Type   string                   `json:"type" binding:"required"` // manager, indexer, both
	Config *WazuhDynamicAuthRequest `json:"config,omitempty"`
}

// WazuhAuthTestResponse 认证测试响应
type WazuhAuthTestResponse struct {
	Type    string                     `json:"type"`
	Results map[string]WazuhTestResult `json:"results"`
	Overall string                     `json:"overall"` // success, partial, failed
}

// WazuhTestResult 单个测试结果
type WazuhTestResult struct {
	Status  string `json:"status"` // success, failed
	Message string `json:"message"`
	Latency string `json:"latency,omitempty"`
}

// WazuhIndicesResponse 索引列表响应
type WazuhIndicesResponse struct {
	Indices []WazuhIndexInfo `json:"indices"`
}

// WazuhIndexInfo 索引信息
type WazuhIndexInfo struct {
	Index        string `json:"index"`
	DocsCount    string `json:"docs.count"`
	StoreSize    string `json:"store.size"`
	CreationDate string `json:"creation.date"`
}

// WazuhClusterHealthResponse 集群健康响应
type WazuhClusterHealthResponse struct {
	ClusterName                 string  `json:"cluster_name"`
	Status                      string  `json:"status"`
	TimedOut                    bool    `json:"timed_out"`
	NumberOfNodes               int     `json:"number_of_nodes"`
	NumberOfDataNodes           int     `json:"number_of_data_nodes"`
	ActivePrimaryShards         int     `json:"active_primary_shards"`
	ActiveShards                int     `json:"active_shards"`
	RelocatingShards            int     `json:"relocating_shards"`
	InitializingShards          int     `json:"initializing_shards"`
	UnassignedShards            int     `json:"unassigned_shards"`
	DelayedUnassignedShards     int     `json:"delayed_unassigned_shards"`
	NumberOfPendingTasks        int     `json:"number_of_pending_tasks"`
	NumberOfInFlightFetch       int     `json:"number_of_in_flight_fetch"`
	TaskMaxWaitingInQueueMillis int     `json:"task_max_waiting_in_queue_millis"`
	ActiveShardsPercentAsNumber float64 `json:"active_shards_percent_as_number"`
}

// WazuhManagerInfoResponse Manager信息响应
type WazuhManagerInfoResponse struct {
	Data    WazuhManagerBasicInfo `json:"data"`
	Message string                `json:"message"`
	Error   int                   `json:"error"`
}

// WazuhManagerBasicInfo 管理器基本信息
type WazuhManagerBasicInfo struct {
	Path           string `json:"path"`
	Version        string `json:"version"`
	Type           string `json:"type"`
	MaxAgents      string `json:"max_agents"`
	OpenSSLSupport string `json:"openssl_support"`
	TZOffset       string `json:"tz_offset"`
	TZName         string `json:"tz_name"`
}

// WazuhManagerStatusResponse Manager状态响应
type WazuhManagerStatusResponse struct {
	Data    WazuhManagerStatus `json:"data"`
	Message string             `json:"message"`
	Error   int                `json:"error"`
}

// WazuhManagerStatus 管理器状态
type WazuhManagerStatus struct {
	WazuhAgentlessD   string `json:"wazuh-agentlessd"`
	WazuhAnalysisD    string `json:"wazuh-analysisd"`
	WazuhAuthD        string `json:"wazuh-authd"`
	WazuhCSyslogD     string `json:"wazuh-csyslogd"`
	WazuhDBD          string `json:"wazuh-dbd"`
	WazuhMonitorD     string `json:"wazuh-monitord"`
	WazuhExecD        string `json:"wazuh-execd"`
	WazuhIntegratorD  string `json:"wazuh-integratord"`
	WazuhLogCollector string `json:"wazuh-logcollector"`
	WazuhMailD        string `json:"wazuh-maild"`
	WazuhRemoteD      string `json:"wazuh-remoted"`
	WazuhReportD      string `json:"wazuh-reportd"`
	WazuhSysCheckD    string `json:"wazuh-syscheckd"`
	WazuhClusterD     string `json:"wazuh-clusterd"`
	WazuhModulesD     string `json:"wazuh-modulesd"`
	WazuhDB           string `json:"wazuh-db"`
	WazuhAPIID        string `json:"wazuh-apid"`
}

// WazuhResponse 通用响应
type WazuhResponse struct {
	Data               interface{} `json:"data,omitempty"`
	Message            string      `json:"message,omitempty"`
	Error              int         `json:"error"`
	Acknowledged       bool        `json:"acknowledged,omitempty"`
	ShardsAcknowledged bool        `json:"shards_acknowledged,omitempty"`
	Index              string      `json:"index,omitempty"`
}

// WazuhEventResponse 单个事件响应
type WazuhEventResponse struct {
	Index  string                 `json:"_index"`
	Type   string                 `json:"_type"`
	ID     string                 `json:"_id"`
	Found  bool                   `json:"found"`
	Source map[string]interface{} `json:"_source"`
}

// WazuhAggregationQuery 聚合查询
type WazuhAggregationQuery struct {
	IndexType     string                   `json:"index_type,omitempty"` // alerts or archives
	StartTime     time.Time                `json:"start_time,omitempty"`
	EndTime       time.Time                `json:"end_time,omitempty"`
	AgentID       string                   `json:"agent_id,omitempty"`
	GroupBy       string                   `json:"group_by,omitempty"`       // 聚合字段
	DateHistogram string                   `json:"date_histogram,omitempty"` // 时间直方图间隔
	Size          int                      `json:"size,omitempty"`
	Metrics       []WazuhAggregationMetric `json:"metrics,omitempty"`
}

// WazuhAggregationMetric 聚合指标
type WazuhAggregationMetric struct {
	Name  string `json:"name"`
	Type  string `json:"type"` // count, sum, avg, max, min
	Field string `json:"field"`
}

// WazuhAggregationResponse 聚合响应
type WazuhAggregationResponse struct {
	Took         int                    `json:"took"`
	TimedOut     bool                   `json:"timed_out"`
	Shards       map[string]interface{} `json:"_shards"`
	Hits         *WazuhSearchHits       `json:"hits"`
	Aggregations map[string]interface{} `json:"aggregations"`
}

// 常量定义
const (
	// Wazuh代理状态
	WazuhAgentStatusActive         = "active"
	WazuhAgentStatusDisconnected   = "disconnected"
	WazuhAgentStatusNeverConnected = "never_connected"
	WazuhAgentStatusPending        = "pending"

	// Wazuh服务状态
	WazuhServiceStatusRunning = "running"
	WazuhServiceStatusStopped = "stopped"

	// 搜索索引类型
	WazuhIndexTypeAlerts   = "alerts"
	WazuhIndexTypeArchives = "archives"

	// 认证测试类型
	WazuhAuthTestTypeManager = "manager"
	WazuhAuthTestTypeIndexer = "indexer"
	WazuhAuthTestTypeBoth    = "both"

	// 配置状态
	WazuhConfigStatusActive   = "active"
	WazuhConfigStatusInactive = "inactive"
	WazuhConfigStatusError    = "error"
)

// WazuhIndexerHealth Wazuh Indexer健康状态
type WazuhIndexerHealth struct {
	ClusterName                 string  `json:"cluster_name"`
	Status                      string  `json:"status"`
	TimedOut                    bool    `json:"timed_out"`
	NumberOfNodes               int     `json:"number_of_nodes"`
	NumberOfDataNodes           int     `json:"number_of_data_nodes"`
	ActivePrimaryShards         int     `json:"active_primary_shards"`
	ActiveShards                int     `json:"active_shards"`
	RelocatingShards            int     `json:"relocating_shards"`
	InitializingShards          int     `json:"initializing_shards"`
	UnassignedShards            int     `json:"unassigned_shards"`
	DelayedUnassignedShards     int     `json:"delayed_unassigned_shards"`
	NumberOfPendingTasks        int     `json:"number_of_pending_tasks"`
	NumberOfInFlightFetch       int     `json:"number_of_in_flight_fetch"`
	TaskMaxWaitingInQueueMillis int     `json:"task_max_waiting_in_queue_millis"`
	ActiveShardsPercentAsNumber float64 `json:"active_shards_percent_as_number"`
}

// WazuhIndexerClusterInfo Wazuh Indexer集群信息
type WazuhIndexerClusterInfo struct {
	Name        string                  `json:"name"`
	ClusterName string                  `json:"cluster_name"`
	ClusterUUID string                  `json:"cluster_uuid"`
	Version     WazuhIndexerVersionInfo `json:"version"`
	Tagline     string                  `json:"tagline"`
}

// WazuhIndexerVersionInfo 版本信息
type WazuhIndexerVersionInfo struct {
	Number                           string    `json:"number"`
	BuildType                        string    `json:"build_type"`
	BuildHash                        string    `json:"build_hash"`
	BuildDate                        time.Time `json:"build_date"`
	BuildSnapshot                    bool      `json:"build_snapshot"`
	LuceneVersion                    string    `json:"lucene_version"`
	MinimumWireCompatibilityVersion  string    `json:"minimum_wire_compatibility_version"`
	MinimumIndexCompatibilityVersion string    `json:"minimum_index_compatibility_version"`
}

// WazuhIndexerIndex 索引信息
type WazuhIndexerIndex struct {
	Index      string `json:"index"`
	Status     string `json:"status"`
	Health     string `json:"health"`
	Pri        string `json:"pri"`
	Rep        string `json:"rep"`
	DocsCount  string `json:"docs.count"`
	StoreSize  string `json:"store.size"`
	UUID       string `json:"uuid,omitempty"`
	CreationDate string `json:"creation.date,omitempty"`
}

// WazuhAgentsSummaryResponse 代理状态汇总响应
type WazuhAgentsSummaryResponse struct {
	Data    WazuhAgentsSummary `json:"data"`
	Message string             `json:"message"`
	Error   int                `json:"error"`
}

// WazuhAgentsSummary 代理状态汇总
type WazuhAgentsSummary struct {
	Connection    WazuhAgentConnectionSummary    `json:"connection"`
	Configuration WazuhAgentConfigurationSummary `json:"configuration"`
}

// WazuhAgentConnectionSummary 代理连接状态汇总
type WazuhAgentConnectionSummary struct {
	Active         int `json:"active"`
	Disconnected   int `json:"disconnected"`
	NeverConnected int `json:"never_connected"`
	Pending        int `json:"pending"`
	Total          int `json:"total"`
}

// WazuhAgentConfigurationSummary 代理配置状态汇总
type WazuhAgentConfigurationSummary struct {
	Synced    int `json:"synced"`
	NotSynced int `json:"not_synced"`
	Total     int `json:"total"`
}

// WazuhGroupListResponse 组列表响应
type WazuhGroupListResponse struct {
	Data    WazuhGroupListData `json:"data"`
	Message string             `json:"message"`
	Error   int                `json:"error"`
}

// WazuhGroupListData 组列表数据
type WazuhGroupListData struct {
	AffectedItems      []*WazuhGroup `json:"affected_items"`
	TotalAffectedItems int           `json:"total_affected_items"`
	TotalFailedItems   int           `json:"total_failed_items"`
	FailedItems        []interface{} `json:"failed_items"`
}

// WazuhGroup 代理组
type WazuhGroup struct {
	Name      string `json:"name"`
	Count     int    `json:"count"`
	MergedSum string `json:"merged_sum"`
	ConfigSum string `json:"config_sum"`
}

// WazuhGroupResponse 组操作响应
type WazuhGroupResponse struct {
	Message string `json:"message,omitempty"`
	Error   int    `json:"error"`
}

// WazuhGroupAgentsResponse 组内代理响应
type WazuhGroupAgentsResponse struct {
	Data    WazuhGroupAgentsData `json:"data"`
	Message string               `json:"message"`
	Error   int                  `json:"error"`
}

// WazuhGroupAgentsData 组内代理数据
type WazuhGroupAgentsData struct {
	AffectedItems      []*WazuhAgent `json:"affected_items"`
	TotalAffectedItems int           `json:"total_affected_items"`
	TotalFailedItems   int           `json:"total_failed_items"`
	FailedItems        []interface{} `json:"failed_items"`
}

// WazuhGroupConfigResponse 组配置响应
type WazuhGroupConfigResponse struct {
	Data    WazuhGroupConfigData `json:"data"`
	Message string               `json:"message,omitempty"`
	Error   int                  `json:"error"`
}

// WazuhGroupConfigData 组配置数据
type WazuhGroupConfigData struct {
	TotalAffectedItems int                    `json:"total_affected_items"`
	AffectedItems      []WazuhGroupConfigItem `json:"affected_items"`
	TotalFailedItems   int                    `json:"total_failed_items,omitempty"`
	FailedItems        []interface{}          `json:"failed_items,omitempty"`
}

// WazuhGroupConfigItem 组配置项
type WazuhGroupConfigItem struct {
	Filters map[string]interface{} `json:"filters"`
	Config  map[string]interface{} `json:"config"`
}

// MaskPassword 脱敏密码显示
func MaskPassword(password string) string {
	if len(password) <= 4 {
		return "****"
	}
	return password[:2] + "****" + password[len(password)-2:]
}
