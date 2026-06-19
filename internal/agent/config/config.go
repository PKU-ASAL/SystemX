package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

type Config struct {
	Agent    AgentConfig
	Manager  ManagerConfig
	Control  ControlConfig
	Sensor   SensorConfig
	Spool    SpoolConfig
	Upload   UploadConfig
	Health   HealthConfig
	Policy   PolicyConfig
	Content  ContentConfig
	Resource ResourceConfig
}

type AgentConfig struct {
	ID       string
	HostID   string
	TenantID string
	Token    string
	Scenario string
	Labels   map[string]string
}

type ManagerConfig struct {
	Address   string
	Transport string
}

type ControlConfig struct {
	SocketPath string
}

type SensorConfig struct {
	Backend           string
	Mode              string
	Version           string
	BundleDir         string
	InstallDir        string
	TetraPath         string
	TetragonPath      string
	EventTransport    string
	ServerAddress     string
	CgroupRate        string
	PprofAddress      string
	GopsAddress       string
	ProcessCacheSize  int
	DataCacheSize     int
	EventQueueSize    int
	RBQueueSize       string
	BTFPath           string
	BPFFSPath         string
	RequireBTF        bool
	RequireBPFFS      bool
	PolicyPath        string
	EventSource       string
	Scope             RuntimeScope
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

type RuntimeScope struct {
	Type     string
	Selector string
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

type PolicyConfig struct {
	RefreshInterval time.Duration
}

type ContentConfig struct {
	Path      string
	TrustKeys string
}

type ResourceConfig struct {
	MaxActiveCEPGroups    int
	MaxEventRefsPerSignal int
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
	if c.Manager.Transport != "local" {
		check("manager.address", c.Manager.Address)
	}
	check("manager.transport", c.Manager.Transport)
	check("sensor.backend", c.Sensor.Backend)
	check("sensor.mode", c.Sensor.Mode)
	check("sensor.policy_path", c.Sensor.PolicyPath)
	check("spool.path", c.Spool.Path)
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	if c.Manager.Transport != "http" && c.Manager.Transport != "grpc" && c.Manager.Transport != "stream" && c.Manager.Transport != "local" {
		return fmt.Errorf("manager.transport must be http, grpc, stream or local")
	}
	if c.Sensor.Backend != "tetragon" && c.Sensor.Backend != "fake" {
		return fmt.Errorf("sensor.backend must be tetragon or fake")
	}
	if c.Sensor.Mode != "managed" && c.Sensor.Mode != "external" {
		return fmt.Errorf("sensor.mode must be managed or external")
	}
	if c.Sensor.EventTransport == "" {
		c.Sensor.EventTransport = "grpc"
	}
	if c.Sensor.EventTransport != "grpc" && c.Sensor.EventTransport != "tetra" {
		return fmt.Errorf("sensor.event_transport must be grpc or tetra")
	}
	if c.Sensor.ProcessCacheSize < 0 {
		return fmt.Errorf("sensor.process_cache_size must be non-negative")
	}
	if c.Sensor.DataCacheSize < 0 {
		return fmt.Errorf("sensor.data_cache_size must be non-negative")
	}
	if c.Sensor.EventQueueSize < 0 {
		return fmt.Errorf("sensor.event_queue_size must be non-negative")
	}
	if c.Sensor.MaxRestarts < 0 {
		return fmt.Errorf("sensor.max_restarts must be non-negative")
	}
	if c.Sensor.FakeStartupEvents < 0 {
		return fmt.Errorf("sensor.fake_startup_events must be non-negative")
	}
	scope, err := c.Sensor.EffectiveScope()
	if err != nil {
		return fmt.Errorf("sensor scope: %w", err)
	}
	scopeSelector := strings.TrimSpace(scope.Selector)
	containerIDPrefix := strings.TrimSpace(c.Sensor.ContainerIDPrefix)
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
	if c.Policy.RefreshInterval < 0 {
		return fmt.Errorf("policy.refresh_interval must be non-negative")
	}
	if c.Resource.MaxActiveCEPGroups < 0 {
		return fmt.Errorf("resource.max_active_cep_groups must be non-negative")
	}
	if c.Resource.MaxEventRefsPerSignal < 0 {
		return fmt.Errorf("resource.max_event_refs_per_signal must be non-negative")
	}
	return nil
}

func (s SensorConfig) EffectiveScope() (RuntimeScope, error) {
	scopeType := strings.TrimSpace(s.Scope.Type)
	scopeSelector := strings.TrimSpace(s.Scope.Selector)
	legacyType := strings.TrimSpace(s.ScopeType)
	legacySelector := strings.TrimSpace(s.ScopeSelector)
	containerIDPrefix := strings.TrimSpace(s.ContainerIDPrefix)

	if scopeType != "" && legacyType != "" && scopeType != legacyType {
		return RuntimeScope{}, fmt.Errorf("sensor.scope.type conflicts with sensor.scope_type")
	}
	if scopeSelector != "" && legacySelector != "" && scopeSelector != legacySelector {
		return RuntimeScope{}, fmt.Errorf("sensor.scope.selector conflicts with sensor.scope_selector")
	}
	if scopeType == "" {
		scopeType = legacyType
	}
	if scopeSelector == "" {
		scopeSelector = legacySelector
	}
	if scopeType == "" && containerIDPrefix != "" {
		scopeType = "container"
	}
	if scopeSelector == "" && containerIDPrefix != "" {
		scopeSelector = containerIDPrefix
	}
	normalizedType, normalizedSelector, err := contract.NormalizeScope(scopeType, scopeSelector)
	if err != nil {
		return RuntimeScope{}, err
	}
	return RuntimeScope{Type: normalizedType, Selector: normalizedSelector}, nil
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
		trimmed := strings.TrimSpace(raw)
		if section == "sensor" && strings.HasSuffix(trimmed, ":") {
			nested := strings.TrimSuffix(trimmed, ":")
			if nested != "scope" {
				return Config{}, fmt.Errorf("line %d: unknown config key sensor.%s", lineNo, nested)
			}
			section = "sensor.scope"
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return Config{}, fmt.Errorf("line %d: expected key: value", lineNo)
		}
		key = strings.TrimSpace(key)
		assignSection := section
		if section == "sensor.scope" && key != "type" && key != "selector" {
			assignSection = "sensor"
		}
		if err := assign(&cfg, assignSection, key, unquote(strings.TrimSpace(value))); err != nil {
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
		Manager:  ManagerConfig{Transport: "stream"},
		Control:  ControlConfig{SocketPath: "/var/run/sysarmor/agent.sock"},
		Sensor:   SensorConfig{Backend: "tetragon", Mode: "managed", EventTransport: "grpc", ServerAddress: "unix:///var/run/tetragon/tetragon.sock", ProcessCacheSize: 4096, DataCacheSize: 128, EventQueueSize: 1024, RBQueueSize: "8192", ObserveOnly: true, Restart: "always", MaxRestarts: 5, RestartWindow: time.Minute},
		Spool:    SpoolConfig{MaxBytes: 256 * 1024 * 1024, BatchSize: 256, FlushInterval: time.Second},
		Upload:   UploadConfig{RetryInitial: time.Second, RetryMax: 30 * time.Second, RequestTimeout: 10 * time.Second},
		Health:   HealthConfig{Interval: 10 * time.Second},
		Policy:   PolicyConfig{RefreshInterval: 30 * time.Second},
		Content:  ContentConfig{Path: "/var/lib/sysarmor/agent/content"},
		Resource: ResourceConfig{MaxActiveCEPGroups: 4096, MaxEventRefsPerSignal: 128},
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
			if labelKey, ok := strings.CutPrefix(key, "label."); ok {
				labelKey = strings.TrimSpace(labelKey)
				if labelKey == "" {
					return fmt.Errorf("agent label key is empty")
				}
				if cfg.Agent.Labels == nil {
					cfg.Agent.Labels = map[string]string{}
				}
				cfg.Agent.Labels[labelKey] = value
				return nil
			}
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
	case "control":
		switch key {
		case "socket_path":
			cfg.Control.SocketPath = value
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
		case "event_transport":
			cfg.Sensor.EventTransport = value
		case "server_address":
			cfg.Sensor.ServerAddress = value
		case "cgroup_rate":
			cfg.Sensor.CgroupRate = value
		case "pprof_address":
			cfg.Sensor.PprofAddress = value
		case "gops_address":
			cfg.Sensor.GopsAddress = value
		case "process_cache_size":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("sensor.process_cache_size: %w", err)
			}
			cfg.Sensor.ProcessCacheSize = v
		case "data_cache_size":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("sensor.data_cache_size: %w", err)
			}
			cfg.Sensor.DataCacheSize = v
		case "event_queue_size":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("sensor.event_queue_size: %w", err)
			}
			cfg.Sensor.EventQueueSize = v
		case "rb_queue_size":
			cfg.Sensor.RBQueueSize = value
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
	case "sensor.scope":
		switch key {
		case "type":
			cfg.Sensor.Scope.Type = value
		case "selector":
			cfg.Sensor.Scope.Selector = value
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
	case "policy":
		switch key {
		case "refresh_interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("policy.refresh_interval: %w", err)
			}
			cfg.Policy.RefreshInterval = d
		default:
			return unknown(section, key)
		}
	case "content":
		switch key {
		case "path":
			cfg.Content.Path = value
		case "trust_keys":
			cfg.Content.TrustKeys = value
		default:
			return unknown(section, key)
		}
	case "resource":
		switch key {
		case "max_active_cep_groups":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("resource.max_active_cep_groups: %w", err)
			}
			cfg.Resource.MaxActiveCEPGroups = v
		case "max_event_refs_per_signal":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("resource.max_event_refs_per_signal: %w", err)
			}
			cfg.Resource.MaxEventRefsPerSignal = v
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
