package health

import "time"

type AgentHealth struct {
	AgentID       string            `json:"agent_id"`
	HostID        string            `json:"host_id"`
	TenantID      string            `json:"tenant_id"`
	Scope         RuntimeScope      `json:"scope"`
	Status        string            `json:"status"`
	PolicyID      string            `json:"policy_id,omitempty"`
	PolicyVersion uint64            `json:"policy_version,omitempty"`
	PolicyMode    string            `json:"policy_mode,omitempty"`
	UptimeSeconds int64             `json:"uptime_seconds"`
	Capability    SensorCapability  `json:"sensor_capability,omitempty"`
	Sensor        SensorHealth      `json:"sensor_health"`
	Queue         QueueHealth       `json:"queue_health"`
	WAL           WALHealth         `json:"wal_health"`
	DataPlane     DataPlaneHealth   `json:"data_plane_health"`
	CEP           CEPHealth         `json:"cep_health"`
	Streams       LocalStreamHealth `json:"stream_health"`
	ObservedAt    time.Time         `json:"observed_at"`
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

type SensorCapability struct {
	Backend         string                         `json:"backend,omitempty"`
	Version         string                         `json:"version,omitempty"`
	SupportsExec    bool                           `json:"supports_exec,omitempty"`
	SupportsConnect bool                           `json:"supports_connect,omitempty"`
	SupportsFile    bool                           `json:"supports_file,omitempty"`
	SupportsEnforce bool                           `json:"supports_enforce,omitempty"`
	SupportsHealth  bool                           `json:"supports_health,omitempty"`
	KernelRelease   string                         `json:"kernel_release,omitempty"`
	BTFAvailable    bool                           `json:"btf_available,omitempty"`
	BPFFSAvailable  bool                           `json:"bpffs_available,omitempty"`
	Collection      []CollectionBehaviorCapability `json:"collection,omitempty"`
}

type CollectionBehaviorCapability struct {
	Behavior             string   `json:"behavior"`
	SensorMapping        string   `json:"sensor_mapping,omitempty"`
	Fields               []string `json:"fields"`
	PushdownSelectors    []string `json:"pushdown_selectors,omitempty"`
	AgentSideSelectors   []string `json:"agent_side_selectors,omitempty"`
	UnsupportedSelectors []string `json:"unsupported_selectors,omitempty"`
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

type WALHealth struct {
	QueuedBatches     int    `json:"queued_batches"`
	QueuedBytes       int64  `json:"queued_bytes"`
	MaxBytes          int64  `json:"max_bytes"`
	OldestBatchID     string `json:"oldest_batch_id,omitempty"`
	NewestBatchID     string `json:"newest_batch_id,omitempty"`
	LastAckedBatchID  string `json:"last_acked_batch_id,omitempty"`
	WatchSubscribers  uint64 `json:"watch_subscribers"`
	BackpressureCount uint64 `json:"backpressure_count"`
	DroppedBatches    uint64 `json:"dropped_batches"`
	DroppedBytes      uint64 `json:"dropped_bytes"`
	LastError         string `json:"last_error,omitempty"`
}

type DataPlaneHealth struct {
	AppendedBatches  int    `json:"appended_batches"`
	RemainingBatches int    `json:"remaining_batches"`
	RemainingBytes   int64  `json:"remaining_bytes"`
	LastError        string `json:"last_error,omitempty"`
}

type CEPHealth struct {
	ActiveGroups     uint64 `json:"active_groups"`
	EvictedGroups    uint64 `json:"evicted_groups"`
	ExpiredGroups    uint64 `json:"expired_groups"`
	DroppedEventRefs uint64 `json:"dropped_event_refs"`
	EvalErrors       uint64 `json:"eval_errors"`
	EmittedSignals   uint64 `json:"emitted_signals"`
	Degraded         bool   `json:"degraded"`
}

type LocalStreamHealth struct {
	EventCapacity        uint64 `json:"event_capacity"`
	EventBuffered        uint64 `json:"event_buffered"`
	EventNextSequence    uint64 `json:"event_next_sequence"`
	EventOldestSequence  uint64 `json:"event_oldest_sequence"`
	EventNewestSequence  uint64 `json:"event_newest_sequence"`
	EventEvicted         uint64 `json:"event_evicted"`
	EventSubscribers     uint64 `json:"event_subscribers"`
	SignalCapacity       uint64 `json:"signal_capacity"`
	SignalBuffered       uint64 `json:"signal_buffered"`
	SignalNextSequence   uint64 `json:"signal_next_sequence"`
	SignalOldestSequence uint64 `json:"signal_oldest_sequence"`
	SignalNewestSequence uint64 `json:"signal_newest_sequence"`
	SignalEvicted        uint64 `json:"signal_evicted"`
	SignalSubscribers    uint64 `json:"signal_subscribers"`
}
