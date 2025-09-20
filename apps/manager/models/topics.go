package models

// TopicConfig Kafka Topic 配置
type TopicConfig struct {
	Name       string `json:"name"`
	Partitions int    `json:"partitions"`
	Retention  string `json:"retention"`
	Purpose    string `json:"purpose"`
}

// SysArmor Kafka Topics 定义
// 采用分区扩展方案，移除数字后缀，通过分区数扩展支持更多collector
const (
	// 原始数据 Topics - sysarmor.raw.*
	// 存储原始数据，如auditd通过rsyslog发回的事件
	TopicRawAudit = "sysarmor.raw.audit"     // auditd 原始数据
	TopicRawOther = "sysarmor.raw.other"     // Vector解析失败的数据预留

	// 处理后事件 Topics - sysarmor.events.*
	// 存储经过处理后的或结构化比较好的数据
	TopicEventsAudit = "sysarmor.events.audit"   // auditd经过convert后形成的类sysdig事件
	TopicEventsSysdig = "sysarmor.events.sysdig" // Sysdig直接发过来的事件

	// 告警 Topics - sysarmor.alerts.*
	// 存储消费sysarmor.events.*后生成的预警事件
	TopicAlerts = "sysarmor.alerts"           // 一般告警事件
	TopicAlertsHigh = "sysarmor.alerts.high" // 高危告警事件
)

// GetTopicConfigs 获取所有 Topic 配置
func GetTopicConfigs() map[string]TopicConfig {
	return map[string]TopicConfig{
		// 原始数据 Topics - sysarmor.raw.*
		TopicRawAudit: {
			Name:       TopicRawAudit,
			Partitions: 32,
			Retention:  "3d",
			Purpose:    "auditd通过rsyslog发回的原始事件",
		},
		TopicRawOther: {
			Name:       TopicRawOther,
			Partitions: 16,
			Retention:  "1d", 
			Purpose:    "Vector解析失败的数据预留",
		},
		
		// 处理后事件 Topics - sysarmor.events.*
		TopicEventsAudit: {
			Name:       TopicEventsAudit,
			Partitions: 32,
			Retention:  "7d",
			Purpose:    "auditd经过convert后形成的类sysdig事件",
		},
		TopicEventsSysdig: {
			Name:       TopicEventsSysdig,
			Partitions: 32,
			Retention:  "7d",
			Purpose:    "Sysdig直接发过来的事件",
		},
		
		// 告警 Topics - sysarmor.alerts.*
		TopicAlerts: {
			Name:       TopicAlerts,
			Partitions: 16,
			Retention:  "30d",
			Purpose:    "消费sysarmor.events.*后生成的一般预警事件",
		},
		TopicAlertsHigh: {
			Name:       TopicAlertsHigh,
			Partitions: 8,
			Retention:  "90d",
			Purpose:    "消费sysarmor.events.*后生成的高危预警事件",
		},
	}
}

// GetDefaultAuditTopic 获取默认的 audit topic
func GetDefaultAuditTopic() string {
	return TopicRawAudit
}

// GetTopicsByCategory 按类别获取 topics
func GetTopicsByCategory() map[string][]string {
	return map[string][]string{
		"raw": {
			TopicRawAudit,
			TopicRawOther,
		},
		"events": {
			TopicEventsAudit,
			TopicEventsSysdig,
		},
		"alerts": {
			TopicAlerts,
			TopicAlertsHigh,
		},
	}
}

// IsValidTopic 检查是否为有效的 SysArmor topic
func IsValidTopic(topic string) bool {
	configs := GetTopicConfigs()
	_, exists := configs[topic]
	return exists
}

// GetTopicPartitions 获取指定 topic 的分区数
func GetTopicPartitions(topic string) int {
	configs := GetTopicConfigs()
	if config, exists := configs[topic]; exists {
		return config.Partitions
	}
	return 16 // 默认分区数
}
