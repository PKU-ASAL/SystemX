package models

import "time"

// RegisterRequest 注册请求
type RegisterRequest struct {
	Hostname       string             `json:"hostname" binding:"required"`
	IPAddress      string             `json:"ip_address" binding:"required"`
	OSType         string             `json:"os_type" binding:"required"`
	OSVersion      string             `json:"os_version" binding:"required"`
	DeploymentType string             `json:"deployment_type" binding:"required"` // 必须字段
	Metadata       *CollectorMetadata `json:"metadata,omitempty"`                 // 可选的元数据
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	Success bool `json:"success"`
	Data    struct {
		CollectorID       string `json:"collector_id"`
		WorkerURL         string `json:"worker_url"`
		ScriptDownloadURL string `json:"script_download_url"`
	} `json:"data"`
}

// CollectorStatus 收集器状态响应
type CollectorStatus struct {
	CollectorID     string             `json:"collector_id"`
	Status          string             `json:"status"`
	Hostname        string             `json:"hostname"`
	IPAddress       string             `json:"ip_address"`
	WorkerAddress   string             `json:"worker_address"`
	KafkaTopic      string             `json:"kafka_topic"`
	Metadata        *CollectorMetadata `json:"metadata,omitempty"`
	LastHeartbeat   *time.Time         `json:"last_heartbeat,omitempty"`
	LastActive      *time.Time         `json:"last_active,omitempty"`
	RealTimeStatus  string             `json:"realtime_status"`
	LastSeenMinutes int                `json:"last_seen_minutes"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
}

// UpdateMetadataRequest 更新元数据请求
type UpdateMetadataRequest struct {
	Metadata *CollectorMetadata `json:"metadata" binding:"required"`
}

// CollectorSearchRequest 搜索请求
type CollectorSearchRequest struct {
	Filters    *CollectorFilters    `json:"filters,omitempty"`
	Pagination *PaginationRequest   `json:"pagination,omitempty"`
	Sort       *SortRequest         `json:"sort,omitempty"`
}

// CollectorFilters 过滤条件
type CollectorFilters struct {
	Tags        []string `json:"tags,omitempty"`        // 标签过滤（OR 关系）
	Group       string   `json:"group,omitempty"`       // 分组过滤
	Environment string   `json:"environment,omitempty"` // 环境过滤
	Owner       string   `json:"owner,omitempty"`       // 负责人过滤
	Status      string   `json:"status,omitempty"`      // 状态过滤
	Region      string   `json:"region,omitempty"`      // 地域过滤
	Purpose     string   `json:"purpose,omitempty"`     // 用途过滤
}

// PaginationRequest 分页请求
type PaginationRequest struct {
	Page  int `json:"page" binding:"min=1"`   // 页码，从1开始
	Limit int `json:"limit" binding:"min=1"`  // 每页数量
}

// SortRequest 排序请求
type SortRequest struct {
	Field string `json:"field"`                           // 排序字段
	Order string `json:"order" binding:"oneof=asc desc"`  // 排序方向
}

// CollectorSearchResponse 搜索响应
type CollectorSearchResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Collectors []*CollectorStatus `json:"collectors"`
		Total      int                `json:"total"`
		Page       int                `json:"page"`
		Limit      int                `json:"limit"`
		TotalPages int                `json:"total_pages"`
	} `json:"data"`
}

// HeartbeatRequest 心跳请求
type HeartbeatRequest struct {
	Status  string `json:"status" binding:"required,oneof=active inactive error offline unregistered"`
	ProbeID string `json:"probe_id,omitempty"` // 可选，用于探测响应
}

// HeartbeatResponse 心跳响应
type HeartbeatResponse struct {
	Success               bool      `json:"success"`
	NextHeartbeatInterval int       `json:"next_heartbeat_interval"`
	ServerTime            time.Time `json:"server_time"`
}

// ProbeRequest 探测请求
type ProbeRequest struct {
	Timeout int `json:"timeout,omitempty"` // 探测超时时间(秒)，默认10秒
}

// ProbeResponse 探测响应
type ProbeResponse struct {
	CollectorID     string     `json:"collector_id"`
	Success         bool       `json:"success"`
	ProbeID         string     `json:"probe_id"`
	SentAt          time.Time  `json:"sent_at"`
	HeartbeatBefore *time.Time `json:"heartbeat_before,omitempty"`
	HeartbeatAfter  *time.Time `json:"heartbeat_after,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
}
