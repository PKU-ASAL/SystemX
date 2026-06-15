package health

import "time"

type AgentHealth struct {
	AgentID       string       `json:"agent_id"`
	HostID        string       `json:"host_id"`
	TenantID      string       `json:"tenant_id"`
	Scope         RuntimeScope `json:"scope"`
	Status        string       `json:"status"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	Sensor        SensorHealth `json:"sensor_health"`
	Queue         QueueHealth  `json:"queue_health"`
	Upload        UploadHealth `json:"upload_health"`
	ObservedAt    time.Time    `json:"observed_at"`
}

type RuntimeScope struct {
	Type     string `json:"type"`
	Selector string `json:"selector,omitempty"`
}

type SensorHealth struct {
	Backend        string    `json:"backend"`
	Installed      bool      `json:"installed"`
	Running        bool      `json:"running"`
	Version        string    `json:"version"`
	PolicyLoaded   bool      `json:"policy_loaded"`
	EventsSeen     uint64    `json:"events_seen"`
	EventsDropped  uint64    `json:"events_dropped"`
	ParseErrors    uint64    `json:"parse_errors"`
	RestartCount   uint64    `json:"restart_count"`
	LastEventAt    time.Time `json:"last_event_at,omitempty"`
	LastExitReason string    `json:"last_exit_reason,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
}

type QueueHealth struct {
	QueuedBatches     int    `json:"queued_batches"`
	QueuedBytes       int64  `json:"queued_bytes"`
	MaxBytes          int64  `json:"max_bytes"`
	BackpressureCount uint64 `json:"backpressure_count"`
	DroppedBatches    uint64 `json:"dropped_batches"`
	DroppedBytes      uint64 `json:"dropped_bytes"`
	LastError         string `json:"last_error,omitempty"`
}

type UploadHealth struct {
	UploadedBatches  int    `json:"uploaded_batches"`
	RemainingBatches int    `json:"remaining_batches"`
	RemainingBytes   int64  `json:"remaining_bytes"`
	LastError        string `json:"last_error,omitempty"`
}
