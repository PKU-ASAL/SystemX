package config

import (
	"fmt"
	"strings"

	sharedconfig "github.com/sysarmor/sysarmor/shared/config"
)

// Config 管理器配置结构，基于共享配置
type Config struct {
	*sharedconfig.ManagerConfig
}

// Load 加载配置
func Load() (*Config, error) {
	managerConfig, err := sharedconfig.LoadManagerConfig()
	if err != nil {
		return nil, err
	}

	return &Config{
		ManagerConfig: managerConfig,
	}, nil
}

// GetKafkaBrokerList 获取 Kafka bootstrap servers 列表
// Manager 通过服务发现获取 Kafka 信息，这里返回默认配置
func (c *Config) GetKafkaBrokerList() []string {
	// 从环境变量获取 Kafka bootstrap servers，如果没有则使用默认值
	kafkaBrokers := sharedconfig.GetEnv("KAFKA_BOOTSTRAP_SERVERS", "kafka:9092")
	return strings.Split(strings.TrimSpace(kafkaBrokers), ",")
}

// GetWorkerList 获取 Worker 列表
// Manager 通过服务发现获取 Worker 信息，这里返回服务名列表
func (c *Config) GetWorkerList() []string {
	return []string{
		c.MiddlewareService + ":http://middleware:8686/health",
		c.ProcessorService + ":http://processor:8081/health", 
		c.IndexerService + ":http://indexer:9200/health",
	}
}

// GetOpenSearchURL 获取 OpenSearch URL
func (c *Config) GetOpenSearchURL() string {
	return sharedconfig.GetEnv("OPENSEARCH_URL", "http://opensearch:9200")
}

// GetOpenSearchUsername 获取 OpenSearch 用户名
func (c *Config) GetOpenSearchUsername() string {
	return sharedconfig.GetEnv("OPENSEARCH_USERNAME", "admin")
}

// GetOpenSearchPassword 获取 OpenSearch 密码
func (c *Config) GetOpenSearchPassword() string {
	return sharedconfig.GetEnv("OPENSEARCH_PASSWORD", "admin")
}

// GetFlinkURL 获取 Flink JobManager URL
func (c *Config) GetFlinkURL() string {
	return sharedconfig.GetEnv("FLINK_JOBMANAGER_URL", "http://flink-jobmanager:8081")
}

// GetDownloadDir 获取下载目录路径
func (c *Config) GetDownloadDir() string {
	return c.DownloadDir
}

// GetManagerURL 获取 Manager URL
func (c *Config) GetManagerURL() string {
	if c.ExternalURL != "" {
		return c.ExternalURL
	}
	return fmt.Sprintf("http://localhost:%d", c.Port)
}

// IsProduction 判断是否为生产环境
func (c *Config) IsProduction() bool {
	return c.BaseConfig.IsProduction()
}

// IsDevelopment 判断是否为开发环境
func (c *Config) IsDevelopment() bool {
	return c.BaseConfig.IsDevelopment()
}

// GetWazuhConfigPath 获取Wazuh配置文件路径
func (c *Config) GetWazuhConfigPath() string {
	return sharedconfig.GetEnv("WAZUH_CONFIG_PATH", "./shared/configs/wazuh.yaml")
}

// IsWazuhEnabled 检查Wazuh是否启用
func (c *Config) IsWazuhEnabled() bool {
	return sharedconfig.GetEnv("WAZUH_ENABLED", "false") == "true"
}

// GetVectorHost 获取 Vector 主机（内部通信地址）
func (c *Config) GetVectorHost() string {
	return c.VectorHost
}

// GetExternalHost 获取外部访问主机地址
func (c *Config) GetExternalHost() string {
	return c.ExternalHost
}

// GetVectorTCPPort 获取 Vector TCP 端口
func (c *Config) GetVectorTCPPort() int {
	return c.VectorTCPPort
}

// GetWazuhConfig 获取Wazuh配置
func (c *Config) GetWazuhConfig() (*WazuhConfig, error) {
	if !c.IsWazuhEnabled() {
		return nil, fmt.Errorf("wazuh plugin is disabled")
	}
	
	configPath := c.GetWazuhConfigPath()
	return LoadWazuhConfig(configPath)
}
