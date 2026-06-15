package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Agent   AgentConfig
	Manager ManagerConfig
	Sensor  SensorConfig
	Spool   SpoolConfig
	Upload  UploadConfig
	Health  HealthConfig
}

type AgentConfig struct {
	ID       string
	HostID   string
	TenantID string
	Token    string
	Scenario string
}

type ManagerConfig struct {
	Address   string
	Transport string
}

type SensorConfig struct {
	Backend           string
	Mode              string
	Version           string
	BundleDir         string
	InstallDir        string
	TetraPath         string
	TetragonPath      string
	BTFPath           string
	BPFFSPath         string
	RequireBTF        bool
	RequireBPFFS      bool
	PolicyPath        string
	EventSource       string
	ScopeType         string
	ScopeSelector     string
	ContainerIDPrefix string
	FakeStartupEvents int
	ObserveOnly       bool
	Restart           string
	MaxRestarts       int
	MaxParseErrors    uint64
	MaxDroppedEvents  uint64
	RestartWindow     time.Duration
}

type SpoolConfig struct {
	Path          string
	MaxBytes      int64
	BatchSize     int
	FlushInterval time.Duration
}

type UploadConfig struct {
	RetryInitial   time.Duration
	RetryMax       time.Duration
	RequestTimeout time.Duration
}

type HealthConfig struct {
	Interval time.Duration
}

func LoadFile(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	cfg, err := parse(f)
	if err != nil {
		return Config{}, err
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	var missing []string
	check := func(path, value string) {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, path)
		}
	}
	check("agent.id", c.Agent.ID)
	check("agent.host_id", c.Agent.HostID)
	check("agent.tenant_id", c.Agent.TenantID)
	check("agent.token", c.Agent.Token)
	check("manager.address", c.Manager.Address)
	check("manager.transport", c.Manager.Transport)
	check("sensor.backend", c.Sensor.Backend)
	check("sensor.mode", c.Sensor.Mode)
	check("sensor.policy_path", c.Sensor.PolicyPath)
	check("spool.path", c.Spool.Path)
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	if c.Manager.Transport != "http" && c.Manager.Transport != "grpc" {
		return fmt.Errorf("manager.transport must be http or grpc")
	}
	if c.Sensor.Backend != "tetragon" && c.Sensor.Backend != "fake" {
		return fmt.Errorf("sensor.backend must be tetragon or fake")
	}
	if c.Sensor.Mode != "managed" && c.Sensor.Mode != "external" {
		return fmt.Errorf("sensor.mode must be managed or external")
	}
	if c.Sensor.MaxRestarts < 0 {
		return fmt.Errorf("sensor.max_restarts must be non-negative")
	}
	if c.Sensor.FakeStartupEvents < 0 {
		return fmt.Errorf("sensor.fake_startup_events must be non-negative")
	}
	scopeType := strings.TrimSpace(c.Sensor.ScopeType)
	scopeSelector := strings.TrimSpace(c.Sensor.ScopeSelector)
	containerIDPrefix := strings.TrimSpace(c.Sensor.ContainerIDPrefix)
	if scopeType == "" {
		scopeType = "host"
	}
	switch scopeType {
	case "host", "container", "cgroup", "namespace", "pod":
	default:
		return fmt.Errorf("sensor.scope_type must be one of host, container, cgroup, namespace, pod")
	}
	if scopeType == "host" {
		if scopeSelector != "" {
			return fmt.Errorf("sensor.scope_selector must be empty when sensor.scope_type=host")
		}
	} else if scopeSelector == "" {
		return fmt.Errorf("sensor.scope_selector is required when sensor.scope_type=%s", scopeType)
	}
	if scopeSelector != "" && containerIDPrefix != "" && scopeSelector != containerIDPrefix {
		return fmt.Errorf("sensor.scope_selector conflicts with sensor.container_id_prefix")
	}
	if c.Spool.MaxBytes <= 0 {
		return fmt.Errorf("spool.max_bytes must be positive")
	}
	if c.Spool.BatchSize <= 0 {
		return fmt.Errorf("spool.batch_size must be positive")
	}
	if c.Spool.FlushInterval <= 0 {
		return fmt.Errorf("spool.flush_interval must be positive")
	}
	if c.Upload.RetryInitial <= 0 || c.Upload.RetryMax <= 0 || c.Upload.RequestTimeout <= 0 {
		return fmt.Errorf("upload retry/request timeouts must be positive")
	}
	if c.Upload.RetryInitial > c.Upload.RetryMax {
		return fmt.Errorf("upload.retry_initial must be <= upload.retry_max")
	}
	if c.Health.Interval <= 0 {
		return fmt.Errorf("health.interval must be positive")
	}
	return nil
}

func parse(r *os.File) (Config, error) {
	cfg := defaults()
	scanner := bufio.NewScanner(r)
	section := ""
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := stripComment(scanner.Text())
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if !strings.HasPrefix(raw, " ") && strings.HasSuffix(strings.TrimSpace(raw), ":") {
			section = strings.TrimSuffix(strings.TrimSpace(raw), ":")
			continue
		}
		if section == "" {
			return Config{}, fmt.Errorf("line %d: key outside section", lineNo)
		}
		key, value, ok := strings.Cut(strings.TrimSpace(raw), ":")
		if !ok {
			return Config{}, fmt.Errorf("line %d: expected key: value", lineNo)
		}
		if err := assign(&cfg, section, strings.TrimSpace(key), unquote(strings.TrimSpace(value))); err != nil {
			return Config{}, fmt.Errorf("line %d: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaults() Config {
	return Config{
		Manager: ManagerConfig{Transport: "http"},
		Sensor:  SensorConfig{Backend: "tetragon", Mode: "managed", ObserveOnly: true, Restart: "always", MaxRestarts: 5, RestartWindow: time.Minute},
		Spool:   SpoolConfig{MaxBytes: 256 * 1024 * 1024, BatchSize: 256, FlushInterval: time.Second},
		Upload:  UploadConfig{RetryInitial: time.Second, RetryMax: 30 * time.Second, RequestTimeout: 10 * time.Second},
		Health:  HealthConfig{Interval: 10 * time.Second},
	}
}

func assign(cfg *Config, section, key, value string) error {
	switch section {
	case "agent":
		switch key {
		case "id":
			cfg.Agent.ID = value
		case "host_id":
			cfg.Agent.HostID = value
		case "tenant_id":
			cfg.Agent.TenantID = value
		case "token":
			cfg.Agent.Token = value
		case "scenario":
			cfg.Agent.Scenario = value
		default:
			return unknown(section, key)
		}
	case "manager":
		switch key {
		case "address":
			cfg.Manager.Address = value
		case "transport":
			cfg.Manager.Transport = value
		default:
			return unknown(section, key)
		}
	case "sensor":
		switch key {
		case "backend":
			cfg.Sensor.Backend = value
		case "mode":
			cfg.Sensor.Mode = value
		case "version":
			cfg.Sensor.Version = value
		case "bundle_dir":
			cfg.Sensor.BundleDir = value
		case "install_dir":
			cfg.Sensor.InstallDir = value
		case "tetra_path":
			cfg.Sensor.TetraPath = value
		case "tetragon_path":
			cfg.Sensor.TetragonPath = value
		case "btf_path":
			cfg.Sensor.BTFPath = value
		case "bpffs_path":
			cfg.Sensor.BPFFSPath = value
		case "require_btf":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("sensor.require_btf: %w", err)
			}
			cfg.Sensor.RequireBTF = b
		case "require_bpffs":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("sensor.require_bpffs: %w", err)
			}
			cfg.Sensor.RequireBPFFS = b
		case "policy_path":
			cfg.Sensor.PolicyPath = value
		case "event_source":
			cfg.Sensor.EventSource = value
		case "scope_type":
			cfg.Sensor.ScopeType = value
		case "scope_selector":
			cfg.Sensor.ScopeSelector = value
		case "container_id_prefix":
			cfg.Sensor.ContainerIDPrefix = value
		case "fake_startup_events":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("sensor.fake_startup_events: %w", err)
			}
			cfg.Sensor.FakeStartupEvents = v
		case "observe_only":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("sensor.observe_only: %w", err)
			}
			cfg.Sensor.ObserveOnly = b
		case "restart":
			cfg.Sensor.Restart = value
		case "max_restarts":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("sensor.max_restarts: %w", err)
			}
			cfg.Sensor.MaxRestarts = v
		case "max_parse_errors":
			v, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return fmt.Errorf("sensor.max_parse_errors: %w", err)
			}
			cfg.Sensor.MaxParseErrors = v
		case "max_dropped_events":
			v, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return fmt.Errorf("sensor.max_dropped_events: %w", err)
			}
			cfg.Sensor.MaxDroppedEvents = v
		case "restart_window":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("sensor.restart_window: %w", err)
			}
			cfg.Sensor.RestartWindow = d
		default:
			return unknown(section, key)
		}
	case "spool":
		switch key {
		case "path":
			cfg.Spool.Path = value
		case "max_bytes":
			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("spool.max_bytes: %w", err)
			}
			cfg.Spool.MaxBytes = v
		case "batch_size":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("spool.batch_size: %w", err)
			}
			cfg.Spool.BatchSize = v
		case "flush_interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("spool.flush_interval: %w", err)
			}
			cfg.Spool.FlushInterval = d
		default:
			return unknown(section, key)
		}
	case "upload":
		switch key {
		case "retry_initial":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("upload.retry_initial: %w", err)
			}
			cfg.Upload.RetryInitial = d
		case "retry_max":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("upload.retry_max: %w", err)
			}
			cfg.Upload.RetryMax = d
		case "request_timeout":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("upload.request_timeout: %w", err)
			}
			cfg.Upload.RequestTimeout = d
		default:
			return unknown(section, key)
		}
	case "health":
		switch key {
		case "interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("health.interval: %w", err)
			}
			cfg.Health.Interval = d
		default:
			return unknown(section, key)
		}
	default:
		return fmt.Errorf("unknown section %q", section)
	}
	return nil
}

func unknown(section, key string) error {
	return fmt.Errorf("unknown config key %s.%s", section, key)
}

func stripComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		return line[:i]
	}
	return line
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
