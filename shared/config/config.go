// Package config provides 12-Factor App configuration management for SysArmor services
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

// BaseConfig contains common configuration for all SysArmor services
type BaseConfig struct {
	Environment string `envconfig:"ENVIRONMENT" default:"development"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"info"`
}

// ManagerConfig contains configuration for the Manager service
type ManagerConfig struct {
	BaseConfig
	Port        int    `envconfig:"MANAGER_PORT" default:"8080"`
	DatabaseURL string `envconfig:"MANAGER_DB_URL" default:"postgres://sysarmor:password@postgres:5432/sysarmor"`

	// Downstream services (discovered via Consul)
	MiddlewareService string `envconfig:"MIDDLEWARE_SERVICE" default:"sysarmor-middleware"`
	ProcessorService  string `envconfig:"PROCESSOR_SERVICE" default:"sysarmor-processor"`
	IndexerService    string `envconfig:"INDEXER_SERVICE" default:"sysarmor-indexer"`

	// OpenSearch configuration (for health checks and API access)
	OpenSearchURL      string `envconfig:"OPENSEARCH_URL" default:"http://opensearch:9200"`
	OpenSearchUsername string `envconfig:"OPENSEARCH_USERNAME" default:"admin"`
	OpenSearchPassword string `envconfig:"OPENSEARCH_PASSWORD" default:"admin"`

	// Resource management
	TemplateDir string `envconfig:"TEMPLATE_DIR" default:"./shared/templates"`
	DownloadDir string `envconfig:"DOWNLOAD_DIR" default:"./shared/binaries"`
	ExternalURL string `envconfig:"EXTERNAL_URL" default:""`
}

// MiddlewareConfig contains configuration for the Middleware service
type MiddlewareConfig struct {
	BaseConfig
	VectorTCPPort         int    `envconfig:"VECTOR_TCP_PORT" default:"6000"`
	VectorAPIPort         int    `envconfig:"VECTOR_API_PORT" default:"8686"`
	KafkaBootstrapServers string `envconfig:"KAFKA_BOOTSTRAP_SERVERS" default:"kafka:9092"`
}

// ProcessorConfig contains configuration for the Processor service
type ProcessorConfig struct {
	BaseConfig
	KafkaBootstrapServers string `envconfig:"KAFKA_BOOTSTRAP_SERVERS" default:"kafka:9092"`
	OpenSearchURL         string `envconfig:"OPENSEARCH_URL" default:"http://opensearch:9200"`
	FlinkJobManagerPort   int    `envconfig:"FLINK_JOBMANAGER_PORT" default:"8081"`
	FlinkTaskManagerSlots int    `envconfig:"FLINK_TASKMANAGER_SLOTS" default:"2"`
	FlinkParallelism      int    `envconfig:"FLINK_PARALLELISM" default:"2"`
	ThreatRulesPath       string `envconfig:"THREAT_RULES_PATH" default:"/app/configs/rules.yaml"`
}

// IndexerConfig contains configuration for the Indexer service
type IndexerConfig struct {
	BaseConfig
	OpenSearchURL      string `envconfig:"OPENSEARCH_URL" default:"http://opensearch:9200"`
	OpenSearchUsername string `envconfig:"OPENSEARCH_USERNAME" default:"admin"`
	OpenSearchPassword string `envconfig:"OPENSEARCH_PASSWORD" default:"admin"`
	IndexPrefix        string `envconfig:"INDEX_PREFIX" default:"sysarmor-events"`
}

// LoadManagerConfig loads Manager service configuration from environment variables
func LoadManagerConfig() (*ManagerConfig, error) {
	var cfg ManagerConfig
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to load manager config: %w", err)
	}
	return &cfg, nil
}

// LoadMiddlewareConfig loads Middleware service configuration from environment variables
func LoadMiddlewareConfig() (*MiddlewareConfig, error) {
	var cfg MiddlewareConfig
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to load middleware config: %w", err)
	}
	return &cfg, nil
}

// LoadProcessorConfig loads Processor service configuration from environment variables
func LoadProcessorConfig() (*ProcessorConfig, error) {
	var cfg ProcessorConfig
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to load processor config: %w", err)
	}
	return &cfg, nil
}

// LoadIndexerConfig loads Indexer service configuration from environment variables
func LoadIndexerConfig() (*IndexerConfig, error) {
	var cfg IndexerConfig
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to load indexer config: %w", err)
	}
	return &cfg, nil
}

// GetEnv gets an environment variable with a default value
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvInt gets an integer environment variable with a default value
func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetEnvBool gets a boolean environment variable with a default value
func GetEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// GetEnvSlice gets a comma-separated environment variable as a slice
func GetEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

// IsDevelopment returns true if running in development environment
func (c *BaseConfig) IsDevelopment() bool {
	return strings.ToLower(c.Environment) == "development"
}

// IsProduction returns true if running in production environment
func (c *BaseConfig) IsProduction() bool {
	return strings.ToLower(c.Environment) == "production"
}

// Validate validates the configuration
func (c *BaseConfig) Validate() error {
	if c.Environment == "" {
		return fmt.Errorf("ENVIRONMENT is required")
	}

	validEnvs := []string{"development", "staging", "production"}
	env := strings.ToLower(c.Environment)
	for _, validEnv := range validEnvs {
		if env == validEnv {
			return nil
		}
	}

	return fmt.Errorf("invalid ENVIRONMENT: %s, must be one of: %v", c.Environment, validEnvs)
}

// Validate validates Manager-specific configuration
func (c *ManagerConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid MANAGER_PORT: %d", c.Port)
	}

	if c.DatabaseURL == "" {
		return fmt.Errorf("MANAGER_DB_URL is required")
	}

	return nil
}

// Validate validates Middleware-specific configuration
func (c *MiddlewareConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.VectorTCPPort <= 0 || c.VectorTCPPort > 65535 {
		return fmt.Errorf("invalid VECTOR_TCP_PORT: %d", c.VectorTCPPort)
	}

	if c.KafkaBootstrapServers == "" {
		return fmt.Errorf("KAFKA_BOOTSTRAP_SERVERS is required")
	}

	return nil
}

// Validate validates Processor-specific configuration
func (c *ProcessorConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.KafkaBootstrapServers == "" {
		return fmt.Errorf("KAFKA_BOOTSTRAP_SERVERS is required")
	}

	if c.OpenSearchURL == "" {
		return fmt.Errorf("OPENSEARCH_URL is required")
	}

	if c.FlinkParallelism <= 0 {
		return fmt.Errorf("invalid FLINK_PARALLELISM: %d", c.FlinkParallelism)
	}

	return nil
}

// Validate validates Indexer-specific configuration
func (c *IndexerConfig) Validate() error {
	if err := c.BaseConfig.Validate(); err != nil {
		return err
	}

	if c.OpenSearchURL == "" {
		return fmt.Errorf("OPENSEARCH_URL is required")
	}

	if c.IndexPrefix == "" {
		return fmt.Errorf("INDEX_PREFIX is required")
	}

	return nil
}
