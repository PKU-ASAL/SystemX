package models

import (
	"time"
)

// Alert 标准化告警数据结构
type Alert struct {
	// OpenSearch 标准主时间字段
	Timestamp time.Time `json:"@timestamp" bson:"@timestamp"`

	// 告警核心信息
	Alert AlertInfo `json:"alert" bson:"alert"`

	// 原始事件数据
	Event EventData `json:"event" bson:"event"`

	// 时间信息
	Timing TimingInfo `json:"timing" bson:"timing"`

	// 元数据信息
	Metadata MetadataInfo `json:"metadata" bson:"metadata"`
}

// AlertInfo 告警详细信息
type AlertInfo struct {
	ID         string      `json:"id" bson:"id"`
	Type       string      `json:"type" bson:"type"`
	Category   string      `json:"category" bson:"category"`
	Severity   string      `json:"severity" bson:"severity"`
	RiskScore  int         `json:"risk_score" bson:"risk_score"`
	Confidence float64     `json:"confidence" bson:"confidence"`
	Rule       RuleInfo    `json:"rule" bson:"rule"`
	Evidence   EvidenceInfo `json:"evidence" bson:"evidence"`
}

// RuleInfo 规则信息
type RuleInfo struct {
	ID          string   `json:"id" bson:"id"`
	Name        string   `json:"name" bson:"name"`
	Description string   `json:"description" bson:"description"`
	Title       string   `json:"title" bson:"title"`
	Mitigation  string   `json:"mitigation" bson:"mitigation"`
	References  []string `json:"references" bson:"references"`
}

// EvidenceInfo 证据信息
type EvidenceInfo struct {
	EventType     string                 `json:"event_type" bson:"event_type"`
	ProcessName   string                 `json:"process_name" bson:"process_name"`
	ProcessCmdline string                `json:"process_cmdline" bson:"process_cmdline"`
	FilePath      string                 `json:"file_path" bson:"file_path"`
	NetworkInfo   map[string]interface{} `json:"network_info" bson:"network_info"`
}

// EventData 事件数据
type EventData struct {
	Raw RawEventData `json:"raw" bson:"raw"`
}

// RawEventData 原始事件数据
type RawEventData struct {
	EventID   string                 `json:"event_id" bson:"event_id"`
	Timestamp time.Time              `json:"timestamp" bson:"timestamp"`
	Source    string                 `json:"source" bson:"source"`
	Message   map[string]interface{} `json:"message" bson:"message"` // 包含完整的 sysdig 数据
}

// TimingInfo 时间信息
type TimingInfo struct {
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`
	ProcessedAt time.Time `json:"processed_at" bson:"processed_at"`
}

// MetadataInfo 元数据信息
type MetadataInfo struct {
	CollectorID string `json:"collector_id" bson:"collector_id"`
	Host        string `json:"host" bson:"host"`
	Source      string `json:"source" bson:"source"`
	Processor   string `json:"processor" bson:"processor"`
}

// SourceEventRef 源事件引用
type SourceEventRef struct {
	EventID   string    `json:"event_id" bson:"event_id"`
	Topic     string    `json:"topic" bson:"topic"`
	Timestamp time.Time `json:"timestamp" bson:"timestamp"`
	EventType string    `json:"event_type" bson:"event_type"`
}

// AlertSeverity 告警严重程度枚举
type AlertSeverity string

const (
	SeverityLow      AlertSeverity = "low"
	SeverityMedium   AlertSeverity = "medium"
	SeverityHigh     AlertSeverity = "high"
	SeverityCritical AlertSeverity = "critical"
)

// AlertType 告警类型枚举
type AlertType string

const (
	AlertTypeRuleBased    AlertType = "rule_based_detection"
	AlertTypeAnomaly      AlertType = "anomaly_detection"
	AlertTypeBehavioral   AlertType = "behavioral_analysis"
	AlertTypeSignature    AlertType = "signature_based"
)

// AlertCategory 告警类别枚举
type AlertCategory string

const (
	CategoryNetworkConnections AlertCategory = "network_connections"
	CategoryPrivilegeEscalation AlertCategory = "privilege_escalation"
	CategorySuspiciousActivity  AlertCategory = "suspicious_activity"
	CategoryFileOperations     AlertCategory = "file_operations"
	CategoryProcessExecution   AlertCategory = "process_execution"
	CategorySystemModification AlertCategory = "system_modification"
)

// NewAlert 创建新的告警实例
func NewAlert(alertID string, ruleID string, severity AlertSeverity) *Alert {
	now := time.Now().UTC()
	
	return &Alert{
		Timestamp: now,
		Alert: AlertInfo{
			ID:         alertID,
			Type:       string(AlertTypeRuleBased),
			Severity:   string(severity),
			Confidence: 0.8,
			Rule: RuleInfo{
				ID: ruleID,
			},
		},
		Timing: TimingInfo{
			CreatedAt:   now,
			ProcessedAt: now,
		},
	}
}

// IsHighSeverity 判断是否为高严重程度告警
func (a *Alert) IsHighSeverity() bool {
	return a.Alert.Severity == string(SeverityHigh) || a.Alert.Severity == string(SeverityCritical)
}

// GetRiskLevel 获取风险等级
func (a *Alert) GetRiskLevel() string {
	switch {
	case a.Alert.RiskScore >= 90:
		return "critical"
	case a.Alert.RiskScore >= 70:
		return "high"
	case a.Alert.RiskScore >= 50:
		return "medium"
	default:
		return "low"
	}
}
