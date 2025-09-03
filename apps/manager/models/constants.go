package models

// CollectorStatus 状态常量
const (
	CollectorStatusActive      = "active"
	CollectorStatusInactive    = "inactive"
	CollectorStatusError       = "error"
	CollectorStatusUnregistered = "unregistered"
)

// WorkerStatus 状态常量
const (
	WorkerStatusActive    = "active"
	WorkerStatusInactive  = "inactive"
	WorkerStatusUnhealthy = "unhealthy"
)

// TopicStatus 状态常量
const (
	TopicStatusActive   = "active"
	TopicStatusInactive = "inactive"
	TopicStatusDeleted  = "deleted"
)

// DeploymentType 部署类型常量
const (
	DeploymentTypeAgentless = "agentless"      // 无侵入方案
	DeploymentTypeSysArmor  = "sysarmor-stack" // SysArmor 全栈方案
	DeploymentTypeWazuh     = "wazuh-hybrid"   // Wazuh 混合方案
)
