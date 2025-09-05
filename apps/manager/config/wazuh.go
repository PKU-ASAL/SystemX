package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// WazuhConfig Wazuh集成配置
type WazuhConfig struct {
	Wazuh WazuhSettings `yaml:"wazuh"`
}

// WazuhSettings Wazuh设置
type WazuhSettings struct {
	Manager  WazuhManagerConfig  `yaml:"manager"`
	Indexer  WazuhIndexerConfig  `yaml:"indexer"`
	Features WazuhFeaturesConfig `yaml:"features"`
}

// WazuhManagerConfig Wazuh Manager配置
type WazuhManagerConfig struct {
	URL         string        `yaml:"url"`
	Username    string        `yaml:"username"`
	Password    string        `yaml:"password"`
	Timeout     time.Duration `yaml:"timeout"`
	TokenExpiry time.Duration `yaml:"token_expiry"`
	TLSVerify   bool          `yaml:"tls_verify"`
	MaxRetries  int           `yaml:"max_retries"`
}

// WazuhIndexerConfig Wazuh Indexer配置
type WazuhIndexerConfig struct {
	URL        string                    `yaml:"url"`
	Host       string                    `yaml:"host"`
	Port       int                       `yaml:"port"`
	Username   string                    `yaml:"username"`
	Password   string                    `yaml:"password"`
	Timeout    time.Duration             `yaml:"timeout"`
	TLS        bool                      `yaml:"tls"`
	TLSVerify  bool                      `yaml:"tls_verify"`
	MaxRetries int                       `yaml:"max_retries"`
	Indices    WazuhIndexerIndicesConfig `yaml:"indices"`
}

// WazuhIndexerIndicesConfig 索引配置
type WazuhIndexerIndicesConfig struct {
	Alerts   string `yaml:"alerts"`
	Archives string `yaml:"archives"`
}

// WazuhFeaturesConfig 功能开关配置
type WazuhFeaturesConfig struct {
	ManagerEnabled bool          `yaml:"manager_enabled"`
	IndexerEnabled bool          `yaml:"indexer_enabled"`
	AutoSync       bool          `yaml:"auto_sync"`
	SyncInterval   time.Duration `yaml:"sync_interval"`
}

// LoadWazuhConfig 加载Wazuh配置
func LoadWazuhConfig(configPath string) (*WazuhConfig, error) {
	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("wazuh config file not found: %s", configPath)
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read wazuh config file: %w", err)
	}

	// 解析YAML配置
	var config WazuhConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse wazuh config: %w", err)
	}

	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid wazuh config: %w", err)
	}

	return &config, nil
}

// Validate 验证Wazuh配置
func (c *WazuhConfig) Validate() error {
	// 验证Manager配置
	if c.Wazuh.Features.ManagerEnabled {
		if c.Wazuh.Manager.URL == "" {
			return fmt.Errorf("wazuh manager URL is required")
		}
		if c.Wazuh.Manager.Username == "" {
			return fmt.Errorf("wazuh manager username is required")
		}
		if c.Wazuh.Manager.Password == "" {
			return fmt.Errorf("wazuh manager password is required")
		}
		if c.Wazuh.Manager.Timeout <= 0 {
			c.Wazuh.Manager.Timeout = 30 * time.Second
		}
		if c.Wazuh.Manager.TokenExpiry <= 0 {
			c.Wazuh.Manager.TokenExpiry = 7200 * time.Second
		}
		if c.Wazuh.Manager.MaxRetries <= 0 {
			c.Wazuh.Manager.MaxRetries = 3
		}
	}

	// 验证Indexer配置
	if c.Wazuh.Features.IndexerEnabled {
		if c.Wazuh.Indexer.URL == "" {
			return fmt.Errorf("wazuh indexer URL is required")
		}
		if c.Wazuh.Indexer.Username == "" {
			return fmt.Errorf("wazuh indexer username is required")
		}
		if c.Wazuh.Indexer.Password == "" {
			return fmt.Errorf("wazuh indexer password is required")
		}
		if c.Wazuh.Indexer.Timeout <= 0 {
			c.Wazuh.Indexer.Timeout = 30 * time.Second
		}
		if c.Wazuh.Indexer.MaxRetries <= 0 {
			c.Wazuh.Indexer.MaxRetries = 3
		}
		if c.Wazuh.Indexer.Indices.Alerts == "" {
			c.Wazuh.Indexer.Indices.Alerts = "wazuh-alerts-*"
		}
		if c.Wazuh.Indexer.Indices.Archives == "" {
			c.Wazuh.Indexer.Indices.Archives = "wazuh-archives-*"
		}
	}

	// 验证功能开关
	if c.Wazuh.Features.SyncInterval <= 0 {
		c.Wazuh.Features.SyncInterval = 300 * time.Second
	}

	return nil
}

// IsManagerEnabled 检查Manager是否启用
func (c *WazuhConfig) IsManagerEnabled() bool {
	return c.Wazuh.Features.ManagerEnabled
}

// IsIndexerEnabled 检查Indexer是否启用
func (c *WazuhConfig) IsIndexerEnabled() bool {
	return c.Wazuh.Features.IndexerEnabled
}

// GetManagerConfig 获取Manager配置
func (c *WazuhConfig) GetManagerConfig() WazuhManagerConfig {
	return c.Wazuh.Manager
}

// GetIndexerConfig 获取Indexer配置
func (c *WazuhConfig) GetIndexerConfig() WazuhIndexerConfig {
	return c.Wazuh.Indexer
}

// GetFeaturesConfig 获取功能配置
func (c *WazuhConfig) GetFeaturesConfig() WazuhFeaturesConfig {
	return c.Wazuh.Features
}
