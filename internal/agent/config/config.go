package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/sensors/contract"
)

type Config struct {
	Agent     AgentConfig
	Manager   ManagerConfig
	Control   ControlConfig
	Runtime   RuntimeConfig
	Sensor    SensorConfig
	Telemetry TelemetryConfig
	DataPlane DataPlaneConfig
	Health    HealthConfig
	Policy    PolicyConfig
	Content   ContentConfig
	Resource  ResourceConfig
}

type AgentConfig struct {
	ID       string
	HostID   string
	TenantID string
	Token    string
	Labels   map[string]string
}

type ManagerConfig struct {
	Address       string
	Transport     string
	TLSCA         string
	TLSCert       string
	TLSKey        string
	TLSServerName string
	TLSInsecure   bool
}

type ControlConfig struct {
	SocketPath string
}

type RuntimeConfig struct {
	FeatureFlags RuntimeFeatureFlags
}

type RuntimeFeatureFlags struct {
	MatcherStrategy string
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

type TelemetryConfig struct {
	BatchSize     int
	MaxBytes      int
	FlushInterval time.Duration
}

type DataPlaneConfig struct {
	RetryInitial   time.Duration
	RetryMax       time.Duration
	RequestTimeout time.Duration
	MaxInflight    int
	Compression    string
	TLSProfile     string
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
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	if c.Manager.Transport != "grpc" && c.Manager.Transport != "local" {
		return fmt.Errorf("manager.transport must be grpc or local")
	}
	if !validMatcherStrategy(c.Runtime.FeatureFlags.MatcherStrategy) {
		return fmt.Errorf("runtime.feature_flags.matcher_strategy must be linear or optimized")
	}
	if (c.Manager.TLSCert == "") != (c.Manager.TLSKey == "") {
		return fmt.Errorf("manager.tls_cert and manager.tls_key must be configured together")
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
	if scope.Type == "namespace" && scope.Selector != "self" {
		return fmt.Errorf("sensor scope: namespace scope selector must be self")
	}
	scopeSelector := strings.TrimSpace(scope.Selector)
	containerIDPrefix := strings.TrimSpace(c.Sensor.ContainerIDPrefix)
	if scopeSelector != "" && containerIDPrefix != "" && scopeSelector != containerIDPrefix {
		return fmt.Errorf("sensor.scope_selector conflicts with sensor.container_id_prefix")
	}
	if c.Telemetry.BatchSize <= 0 {
		return fmt.Errorf("telemetry.batch_size must be positive")
	}
	if c.Telemetry.MaxBytes < 0 {
		return fmt.Errorf("telemetry.max_bytes must be non-negative")
	}
	if c.Telemetry.FlushInterval <= 0 {
		return fmt.Errorf("telemetry.flush_interval must be positive")
	}
	if c.DataPlane.RetryInitial <= 0 || c.DataPlane.RetryMax <= 0 || c.DataPlane.RequestTimeout <= 0 {
		return fmt.Errorf("data_plane retry/request timeouts must be positive")
	}
	if c.DataPlane.RetryInitial > c.DataPlane.RetryMax {
		return fmt.Errorf("data_plane.retry_initial must be <= data_plane.retry_max")
	}
	if c.DataPlane.MaxInflight < 0 {
		return fmt.Errorf("data_plane.max_inflight must be non-negative")
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

func validMatcherStrategy(strategy string) bool {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "", "linear", "optimized":
		return true
	default:
		return false
	}
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
		if section == "runtime" && strings.HasSuffix(trimmed, ":") {
			nested := strings.TrimSuffix(trimmed, ":")
			if nested != "feature_flags" {
				return Config{}, fmt.Errorf("line %d: unknown config key runtime.%s", lineNo, nested)
			}
			section = "runtime.feature_flags"
			continue
		}
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
		Manager:   ManagerConfig{Transport: "grpc"},
		Control:   ControlConfig{SocketPath: "/run/sysarmor/agent.sock"},
		Runtime:   RuntimeConfig{FeatureFlags: RuntimeFeatureFlags{MatcherStrategy: "linear"}},
		Sensor:    SensorConfig{Backend: "tetragon", Mode: "managed", EventTransport: "grpc", ServerAddress: "unix:///var/run/tetragon/tetragon.sock", ProcessCacheSize: 4096, DataCacheSize: 128, EventQueueSize: 1024, RBQueueSize: "8192", ObserveOnly: true, Restart: "always", MaxRestarts: 5, RestartWindow: time.Minute},
		Telemetry: TelemetryConfig{BatchSize: 256, MaxBytes: 256 * 1024, FlushInterval: time.Second},
		DataPlane: DataPlaneConfig{RetryInitial: time.Second, RetryMax: 30 * time.Second, RequestTimeout: 10 * time.Second, MaxInflight: 1, Compression: "none"},
		Health:    HealthConfig{Interval: 10 * time.Second},
		Policy:    PolicyConfig{RefreshInterval: 30 * time.Second},
		Content:   ContentConfig{Path: "/var/lib/sysarmor/agent/content"},
		Resource:  ResourceConfig{MaxActiveCEPGroups: 4096, MaxEventRefsPerSignal: 128},
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
		case "tls_ca":
			cfg.Manager.TLSCA = value
		case "tls_cert":
			cfg.Manager.TLSCert = value
		case "tls_key":
			cfg.Manager.TLSKey = value
		case "tls_server_name":
			cfg.Manager.TLSServerName = value
		case "tls_insecure":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("manager.tls_insecure: %w", err)
			}
			cfg.Manager.TLSInsecure = b
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
	case "runtime.feature_flags":
		switch key {
		case "matcher_strategy":
			cfg.Runtime.FeatureFlags.MatcherStrategy = value
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
	case "telemetry":
		switch key {
		case "batch_size":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("telemetry.batch_size: %w", err)
			}
			cfg.Telemetry.BatchSize = v
		case "max_bytes":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("telemetry.max_bytes: %w", err)
			}
			cfg.Telemetry.MaxBytes = v
		case "flush_interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("telemetry.flush_interval: %w", err)
			}
			cfg.Telemetry.FlushInterval = d
		default:
			return unknown(section, key)
		}
	case "data_plane":
		switch key {
		case "retry_initial":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("data_plane.retry_initial: %w", err)
			}
			cfg.DataPlane.RetryInitial = d
		case "retry_max":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("data_plane.retry_max: %w", err)
			}
			cfg.DataPlane.RetryMax = d
		case "request_timeout":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("data_plane.request_timeout: %w", err)
			}
			cfg.DataPlane.RequestTimeout = d
		case "max_inflight":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("data_plane.max_inflight: %w", err)
			}
			cfg.DataPlane.MaxInflight = v
		case "compression":
			cfg.DataPlane.Compression = value
		case "tls_profile":
			cfg.DataPlane.TLSProfile = value
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
