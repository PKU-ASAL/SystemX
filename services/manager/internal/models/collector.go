package models

import (
	"time"

	"github.com/google/uuid"
)

// CollectorMetadata 表示 Collector 的元数据信息
type CollectorMetadata struct {
	Tags        []string `json:"tags,omitempty"`        // 标签列表，如 ["web", "production", "critical"]
	Group       string   `json:"group,omitempty"`       // 分组名称，如 "web-servers", "db-cluster"
	Environment string   `json:"environment,omitempty"` // 环境标识，如 "prod", "staging", "dev"
	Owner       string   `json:"owner,omitempty"`       // 负责人/团队
	Description string   `json:"description,omitempty"` // 描述信息
	Region      string   `json:"region,omitempty"`      // 地域信息，如 "us-east-1", "beijing"
	Datacenter  string   `json:"datacenter,omitempty"`  // 数据中心，如 "dc1", "aws-us-east"
	Purpose     string   `json:"purpose,omitempty"`     // 用途，如 "web-server", "database", "cache"
}

// Collector 表示一个数据收集器
type Collector struct {
	ID             uuid.UUID          `json:"id" db:"id"`
	CollectorID    string             `json:"collector_id" db:"collector_id"`
	Hostname       string             `json:"hostname" db:"hostname"`
	IPAddress      string             `json:"ip_address" db:"ip_address"`
	OSType         string             `json:"os_type" db:"os_type"`
	OSVersion      string             `json:"os_version" db:"os_version"`
	Status         string             `json:"status" db:"status"`
	WorkerAddress  string             `json:"worker_address" db:"worker_address"`
	KafkaTopic     string             `json:"kafka_topic" db:"kafka_topic"`
	DeploymentType string             `json:"deployment_type" db:"deployment_type"`
	Metadata       *CollectorMetadata `json:"metadata,omitempty" db:"metadata"`
	LastHeartbeat  *time.Time         `json:"last_heartbeat,omitempty" db:"last_heartbeat"`
	CreatedAt      time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at" db:"updated_at"`
}

// Collector 辅助方法

// IsActive 判断 Collector 是否活跃
func (c *Collector) IsActive() bool {
	return c.Status == CollectorStatusActive
}

// GetTopicName 生成基于部署类型的 Topic 名称
func (c *Collector) GetTopicName() string {
	var prefix string
	switch c.DeploymentType {
	case DeploymentTypeAgentless:
		prefix = "sysarmor-agentless"
	case DeploymentTypeSysArmor:
		prefix = "sysarmor-stack"
	case DeploymentTypeWazuh:
		prefix = "sysarmor-wazuh"
	default:
		// 不支持的部署类型，使用 agentless 作为默认
		prefix = "sysarmor-agentless"
	}
	
	// 使用 collector_id 的前8位作为后缀
	if len(c.CollectorID) >= 8 {
		return prefix + "-" + c.CollectorID[:8]
	}
	return prefix + "-" + c.CollectorID
}

// GetDeploymentTypeDisplayName 获取部署类型的显示名称
func (c *Collector) GetDeploymentTypeDisplayName() string {
	switch c.DeploymentType {
	case DeploymentTypeAgentless:
		return "Agentless (无侵入)"
	case DeploymentTypeSysArmor:
		return "SysArmor Stack (全栈)"
	case DeploymentTypeWazuh:
		return "Wazuh Hybrid (混合)"
	default:
		return "Unknown"
	}
}

// GetGroup 获取分组名称，如果未设置则返回默认值
func (c *Collector) GetGroup() string {
	if c.Metadata != nil && c.Metadata.Group != "" {
		return c.Metadata.Group
	}
	return "default"
}

// GetEnvironment 获取环境标识，如果未设置则返回默认值
func (c *Collector) GetEnvironment() string {
	if c.Metadata != nil && c.Metadata.Environment != "" {
		return c.Metadata.Environment
	}
	return "production"
}

// HasTag 检查是否包含指定标签
func (c *Collector) HasTag(tag string) bool {
	if c.Metadata == nil || c.Metadata.Tags == nil {
		return false
	}
	for _, t := range c.Metadata.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// AddTag 添加标签（去重）
func (c *Collector) AddTag(tag string) {
	if c.Metadata == nil {
		c.Metadata = &CollectorMetadata{}
	}
	if c.Metadata.Tags == nil {
		c.Metadata.Tags = []string{}
	}
	
	// 检查是否已存在
	for _, t := range c.Metadata.Tags {
		if t == tag {
			return
		}
	}
	
	c.Metadata.Tags = append(c.Metadata.Tags, tag)
}

// RemoveTag 移除标签
func (c *Collector) RemoveTag(tag string) {
	if c.Metadata == nil || c.Metadata.Tags == nil {
		return
	}
	
	var newTags []string
	for _, t := range c.Metadata.Tags {
		if t != tag {
			newTags = append(newTags, t)
		}
	}
	c.Metadata.Tags = newTags
}

// SetMetadata 设置元数据
func (c *Collector) SetMetadata(metadata *CollectorMetadata) {
	c.Metadata = metadata
	c.UpdatedAt = time.Now()
}

// GetDisplayName 获取显示名称（优先使用描述，否则使用主机名）
func (c *Collector) GetDisplayName() string {
	if c.Metadata != nil && c.Metadata.Description != "" {
		return c.Metadata.Description
	}
	return c.Hostname
}
