package opensearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenSearchService OpenSearch 管理服务
type OpenSearchService struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewOpenSearchService 创建 OpenSearch 服务
func NewOpenSearchService(baseURL, username, password string) *OpenSearchService {
	// 创建 HTTP 客户端，跳过 SSL 验证（开发环境）
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	
	return &OpenSearchService{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		username: username,
		password: password,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
		},
	}
}

// ClusterHealth 集群健康状态
type ClusterHealth struct {
	ClusterName                 string  `json:"cluster_name"`
	Status                      string  `json:"status"`
	TimedOut                    bool    `json:"timed_out"`
	NumberOfNodes               int     `json:"number_of_nodes"`
	NumberOfDataNodes           int     `json:"number_of_data_nodes"`
	DiscoveredMaster            bool    `json:"discovered_master"`
	DiscoveredClusterManager    bool    `json:"discovered_cluster_manager"`
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

// ClusterStats 集群统计信息
type ClusterStats struct {
	Timestamp   int64       `json:"timestamp"`
	ClusterName string      `json:"cluster_name"`
	ClusterUUID string      `json:"cluster_uuid"`
	Status      string      `json:"status"`
	Indices     IndicesStats `json:"indices"`
	Nodes       NodesStats   `json:"nodes"`
}

// IndicesStats 索引统计
type IndicesStats struct {
	Count  int         `json:"count"`
	Shards ShardsStats `json:"shards"`
	Docs   DocsStats   `json:"docs"`
	Store  StoreStats  `json:"store"`
}

// ShardsStats 分片统计
type ShardsStats struct {
	Total       int         `json:"total"`
	Primaries   int         `json:"primaries"`
	Replication interface{} `json:"replication"` // 修复：使用interface{}处理不同类型
	Index       struct {
		Shards      ShardsDetail `json:"shards"`
		Primaries   ShardsDetail `json:"primaries"`
		Replication interface{}  `json:"replication"` // 修复：使用interface{}处理不同类型
	} `json:"index"`
}

// ShardsDetail 分片详情
type ShardsDetail struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	Avg float64 `json:"avg"`
}

// DocsStats 文档统计
type DocsStats struct {
	Count   int64 `json:"count"`
	Deleted int64 `json:"deleted"`
}

// StoreStats 存储统计
type StoreStats struct {
	SizeInBytes          int64 `json:"size_in_bytes"`
	ReservedInBytes      int64 `json:"reserved_in_bytes"`
	TotalDataSetSizeInBytes int64 `json:"total_data_set_size_in_bytes"`
}

// NodesStats 节点统计
type NodesStats struct {
	Count       NodesCount       `json:"count"`
	Versions    []string         `json:"versions"`
	OS          OSStats          `json:"os"`
	Process     ProcessStats     `json:"process"`
	JVM         JVMStats         `json:"jvm"`
	FileSystem  FileSystemStats  `json:"fs"`
	Plugins     []PluginInfo     `json:"plugins"`
	NetworkTypes NetworkTypes    `json:"network_types"`
}

// NodesCount 节点数量统计
type NodesCount struct {
	Total            int `json:"total"`
	CoordinatingOnly int `json:"coordinating_only"`
	Data             int `json:"data"`
	DataCold         int `json:"data_cold"`
	DataContent      int `json:"data_content"`
	DataFrozen       int `json:"data_frozen"`
	DataHot          int `json:"data_hot"`
	DataWarm         int `json:"data_warm"`
	Ingest           int `json:"ingest"`
	Master           int `json:"master"`
	ML               int `json:"ml"`
	RemoteClusterClient int `json:"remote_cluster_client"`
	Transform        int `json:"transform"`
}

// OSStats 操作系统统计
type OSStats struct {
	AvailableProcessors int                    `json:"available_processors"`
	AllocatedProcessors int                    `json:"allocated_processors"`
	Names               []OSName               `json:"names"`
	PrettyNames         []OSPrettyName         `json:"pretty_names"`
	Architectures       []OSArchitecture       `json:"architectures"`
	Mem                 OSMemStats             `json:"mem"`
}

// OSName 操作系统名称
type OSName struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// OSPrettyName 操作系统友好名称
type OSPrettyName struct {
	PrettyName string `json:"pretty_name"`
	Count      int    `json:"count"`
}

// OSArchitecture 操作系统架构
type OSArchitecture struct {
	Arch  string `json:"arch"`
	Count int    `json:"count"`
}

// OSMemStats 操作系统内存统计
type OSMemStats struct {
	TotalInBytes     int64 `json:"total_in_bytes"`
	AdjustedTotalInBytes int64 `json:"adjusted_total_in_bytes"`
	FreeInBytes      int64 `json:"free_in_bytes"`
	UsedInBytes      int64 `json:"used_in_bytes"`
	FreePercent      int   `json:"free_percent"`
	UsedPercent      int   `json:"used_percent"`
}

// ProcessStats 进程统计
type ProcessStats struct {
	CPU                 ProcessCPUStats `json:"cpu"`
	OpenFileDescriptors ProcessFDStats  `json:"open_file_descriptors"`
}

// ProcessCPUStats 进程 CPU 统计
type ProcessCPUStats struct {
	Percent int `json:"percent"`
}

// ProcessFDStats 进程文件描述符统计
type ProcessFDStats struct {
	Min int `json:"min"`
	Max int `json:"max"`
	Avg int `json:"avg"`
}

// JVMStats JVM 统计
type JVMStats struct {
	MaxUptimeInMillis int64           `json:"max_uptime_in_millis"`
	Versions          []JVMVersion    `json:"versions"`
	Mem               JVMMemStats     `json:"mem"`
	Threads           int64           `json:"threads"`
}

// JVMVersion JVM 版本
type JVMVersion struct {
	Version   string `json:"version"`
	VMName    string `json:"vm_name"`
	VMVersion string `json:"vm_version"`
	VMVendor  string `json:"vm_vendor"`
	BundledJDK bool   `json:"bundled_jdk"`
	UsingBundledJDK bool `json:"using_bundled_jdk"`
	Count     int    `json:"count"`
}

// JVMMemStats JVM 内存统计
type JVMMemStats struct {
	HeapUsedInBytes int64 `json:"heap_used_in_bytes"`
	HeapMaxInBytes  int64 `json:"heap_max_in_bytes"`
}

// FileSystemStats 文件系统统计
type FileSystemStats struct {
	TotalInBytes     int64 `json:"total_in_bytes"`
	FreeInBytes      int64 `json:"free_in_bytes"`
	AvailableInBytes int64 `json:"available_in_bytes"`
}

// PluginInfo 插件信息
type PluginInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	OpenSearchVersion string `json:"opensearch_version"`
	JavaVersion string `json:"java_version"`
	Description string `json:"description"`
	Classname   string `json:"classname"`
	CustomFoldername string `json:"custom_foldername"`
	ExtendedPlugins []string `json:"extended_plugins"`
	HasNativeController bool `json:"has_native_controller"`
}

// NetworkTypes 网络类型
type NetworkTypes struct {
	TransportTypes map[string]int `json:"transport_types"`
	HTTPTypes      map[string]int `json:"http_types"`
}

// IndexInfo 索引信息
type IndexInfo struct {
	Health       string `json:"health"`
	Status       string `json:"status"`
	Index        string `json:"index"`
	UUID         string `json:"uuid"`
	Pri          string `json:"pri"`
	Rep          string `json:"rep"`
	DocsCount    string `json:"docs.count"`
	DocsDeleted  string `json:"docs.deleted"`
	StoreSize    string `json:"store.size"`
	PriStoreSize string `json:"pri.store.size"`
}

// SearchRequest 搜索请求
type SearchRequest struct {
	Query interface{} `json:"query,omitempty"`
	Size  int         `json:"size,omitempty"`
	From  int         `json:"from,omitempty"`
	Sort  interface{} `json:"sort,omitempty"`
	Aggs  interface{} `json:"aggs,omitempty"`
}

// SearchResponse 搜索响应
type SearchResponse struct {
	Took     int  `json:"took"`
	TimedOut bool `json:"timed_out"`
	Shards   struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Skipped    int `json:"skipped"`
		Failed     int `json:"failed"`
	} `json:"_shards"`
	Hits struct {
		Total struct {
			Value    int64  `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		MaxScore *float64 `json:"max_score"`
		Hits     []struct {
			Index  string                 `json:"_index"`
			Type   string                 `json:"_type"`
			ID     string                 `json:"_id"`
			Score  *float64               `json:"_score"`
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]interface{} `json:"aggregations,omitempty"`
}

// GetClusterHealth 获取集群健康状态
func (s *OpenSearchService) GetClusterHealth(ctx context.Context) (*ClusterHealth, error) {
	url := fmt.Sprintf("%s/_cluster/health", s.baseURL)
	
	resp, err := s.makeRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster health: %w", err)
	}
	defer resp.Body.Close()
	
	var health ClusterHealth
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, fmt.Errorf("failed to decode cluster health response: %w", err)
	}
	
	return &health, nil
}

// GetClusterStats 获取集群统计信息
func (s *OpenSearchService) GetClusterStats(ctx context.Context) (*ClusterStats, error) {
	url := fmt.Sprintf("%s/_cluster/stats", s.baseURL)
	
	resp, err := s.makeRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster stats: %w", err)
	}
	defer resp.Body.Close()
	
	var stats ClusterStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode cluster stats response: %w", err)
	}
	
	return &stats, nil
}

// GetIndices 获取索引列表
func (s *OpenSearchService) GetIndices(ctx context.Context) ([]IndexInfo, error) {
	url := fmt.Sprintf("%s/_cat/indices?format=json&h=health,status,index,uuid,pri,rep,docs.count,docs.deleted,store.size,pri.store.size", s.baseURL)
	
	resp, err := s.makeRequest(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get indices: %w", err)
	}
	defer resp.Body.Close()
	
	var indices []IndexInfo
	if err := json.NewDecoder(resp.Body).Decode(&indices); err != nil {
		return nil, fmt.Errorf("failed to decode indices response: %w", err)
	}
	
	return indices, nil
}

// SearchEvents 搜索安全事件
func (s *OpenSearchService) SearchEvents(ctx context.Context, indexPattern string, request *SearchRequest) (*SearchResponse, error) {
	url := fmt.Sprintf("%s/%s/_search", s.baseURL, indexPattern)
	
	var body []byte
	var err error
	if request != nil {
		body, err = json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal search request: %w", err)
		}
	}
	
	resp, err := s.makeRequest(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to search events: %w", err)
	}
	defer resp.Body.Close()
	
	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}
	
	return &searchResp, nil
}

// GetEventsByTimeRange 根据时间范围获取事件
func (s *OpenSearchService) GetEventsByTimeRange(ctx context.Context, indexPattern string, from, to time.Time, size int, page int) (*SearchResponse, error) {
	offset := (page - 1) * size
	
	// 智能选择时间字段
	timestampField := s.getTimestampField(indexPattern)
	
	request := &SearchRequest{
		Query: map[string]interface{}{
			"range": map[string]interface{}{
				timestampField: map[string]interface{}{
					"gte": from.Format(time.RFC3339),
					"lte": to.Format(time.RFC3339),
				},
			},
		},
		Size: size,
		From: offset,
		Sort: []map[string]interface{}{
			{
				timestampField: map[string]string{
					"order": "desc",
				},
			},
		},
	}
	
	return s.SearchEvents(ctx, indexPattern, request)
}

// getTimestampField 根据索引模式智能选择时间字段
func (s *OpenSearchService) getTimestampField(indexPattern string) string {
	// 统一使用标准的 @timestamp 字段
	// 新的告警数据已经包含 @timestamp 字段
	return "@timestamp"
}

// GetEventsByRiskScore 根据风险评分获取事件
func (s *OpenSearchService) GetEventsByRiskScore(ctx context.Context, indexPattern string, minScore int, size int) (*SearchResponse, error) {
	timestampField := s.getTimestampField(indexPattern)
	
	request := &SearchRequest{
		Query: map[string]interface{}{
			"range": map[string]interface{}{
				"risk_score": map[string]interface{}{
					"gte": minScore,
				},
			},
		},
		Size: size,
		Sort: []map[string]interface{}{
			{
				"risk_score": map[string]string{
					"order": "desc",
				},
			},
			{
				timestampField: map[string]string{
					"order": "desc",
				},
			},
		},
	}
	
	return s.SearchEvents(ctx, indexPattern, request)
}

// GetEventsBySource 根据数据源获取事件
func (s *OpenSearchService) GetEventsBySource(ctx context.Context, indexPattern string, dataSource string, size int) (*SearchResponse, error) {
	timestampField := s.getTimestampField(indexPattern)
	
	request := &SearchRequest{
		Query: map[string]interface{}{
			"term": map[string]interface{}{
				"source.keyword": dataSource,
			},
		},
		Size: size,
		Sort: []map[string]interface{}{
			{
				timestampField: map[string]string{
					"order": "desc",
				},
			},
		},
	}
	
	return s.SearchEvents(ctx, indexPattern, request)
}

// GetThreatEvents 获取威胁事件（标记为可疑的事件）
func (s *OpenSearchService) GetThreatEvents(ctx context.Context, indexPattern string, size int) (*SearchResponse, error) {
	timestampField := s.getTimestampField(indexPattern)
	
	request := &SearchRequest{
		Query: map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"severity.keyword": "high",
						},
					},
					{
						"range": map[string]interface{}{
							"risk_score": map[string]interface{}{
								"gte": 70,
							},
						},
					},
				},
				"minimum_should_match": 1,
			},
		},
		Size: size,
		Sort: []map[string]interface{}{
			{
				"risk_score": map[string]string{
					"order": "desc",
				},
			},
			{
				timestampField: map[string]string{
					"order": "desc",
				},
			},
		},
	}
	
	return s.SearchEvents(ctx, indexPattern, request)
}

// GetEventAggregations 获取事件聚合统计
func (s *OpenSearchService) GetEventAggregations(ctx context.Context, indexPattern string) (*SearchResponse, error) {
	request := &SearchRequest{
		Size: 0, // 不返回具体文档，只返回聚合结果
		Aggs: map[string]interface{}{
			"by_data_source": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "data_source.keyword",
					"size":  10,
				},
			},
			"by_severity": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "event.severity.keyword",
					"size":  10,
				},
			},
			"by_event_type": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "event.type.keyword",
					"size":  10,
				},
			},
			"risk_score_stats": map[string]interface{}{
				"stats": map[string]interface{}{
					"field": "risk_score",
				},
			},
			"events_over_time": map[string]interface{}{
				"date_histogram": map[string]interface{}{
					"field":    "@timestamp",
					"interval": "1h",
				},
			},
		},
	}
	
	return s.SearchEvents(ctx, indexPattern, request)
}

// makeRequest 发起 HTTP 请求的通用方法
func (s *OpenSearchService) makeRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// 设置认证
	req.SetBasicAuth(s.username, s.password)
	
	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	
	// 检查响应状态码
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	
	return resp, nil
}
