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
	DeploymentType string             `json:"deployment_type" db:"deployment_type"`
	Metadata       *CollectorMetadata `json:"metadata,omitempty" db:"metadata"`
	LastHeartbeat  *time.Time         `json:"last_heartbeat,omitempty" db:"last_heartbeat"`
	LastActive     *time.Time         `json:"last_active,omitempty" db:"last_active"`
	CreatedAt      time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at" db:"updated_at"`
}

// Collector 辅助方法

// IsActive 判断 Collector 是否活跃
func (c *Collector) IsActive() bool {
	return c.Status == CollectorStatusActive
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

// GetRealTimeStatus 获取基于时间的实时状态
func (c *Collector) GetRealTimeStatus() string {
	now := time.Now()
	
	// 1. 检查最近心跳 (5分钟内)
	if c.LastHeartbeat != nil && now.Sub(*c.LastHeartbeat) <= 5*time.Minute {
		return c.Status // 返回 Collector 上报的状态 (active/inactive/error)
	}
	
	// 2. 检查最近活跃 (30分钟内)
	if c.LastActive != nil && now.Sub(*c.LastActive) <= 30*time.Minute {
		return "inactive" // 最近活跃过但现在无心跳
	}
	
	// 3. 长时间无响应
	return CollectorStatusOffline // 长时间无任何响应
}

// GetLastSeenMinutes 获取最后活跃的分钟数
func (c *Collector) GetLastSeenMinutes() int {
	if c.LastActive == nil {
		return -1 // 从未活跃
	}
	
	now := time.Now()
	minutes := int(now.Sub(*c.LastActive).Minutes())
	return minutes
}
