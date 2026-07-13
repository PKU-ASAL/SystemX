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
	Local     LocalConfig
	Manager   ManagerConfig
	Control   ControlConfig
	Runtime   RuntimeConfig
	Sensor    SensorConfig
	Telemetry TelemetryConfig
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

type LocalConfig struct {
	StatePath string
	Storage   LocalStorageConfig
	Export    LocalExportConfig
}

type LocalStorageConfig struct {
	MaxBytes       int64
	MinFreeBytes   int64
	SegmentSize    int64
	SignalMaxCount int64
}

type LocalExportConfig struct {
	RetryInitial    time.Duration
	RetryMax        time.Duration
	RequestTimeout  time.Duration
	MaxInflight     int
	WireCompression string
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
	MaxBatchItems int
	MaxBatchBytes int
	FlushInterval time.Duration
}

type HealthConfig struct {
	Interval time.Duration
}

type PolicyConfig struct {
	Path string
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
	check("local.state_path", c.Local.StatePath)
	if c.Manager.Transport == "local" {
		return fmt.Errorf("manager.transport local is legacy; omit manager configuration for standalone mode")
	}
	if c.Manager.Transport == "grpc" {
		check("manager.address", c.Manager.Address)
	}
	check("sensor.backend", c.Sensor.Backend)
	check("sensor.mode", c.Sensor.Mode)
	check("policy.path", c.Policy.Path)
	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}
	if c.Manager.Transport != "" && c.Manager.Transport != "grpc" {
		return fmt.Errorf("manager.transport must be grpc when configured")
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
	if _, err := ResolveTelemetry(c.Telemetry, nil); err != nil {
		return err
	}
	if c.Local.Export.RetryInitial <= 0 || c.Local.Export.RetryMax <= 0 || c.Local.Export.RequestTimeout <= 0 {
		return fmt.Errorf("local.export retry/request timeouts must be positive")
	}
	if c.Local.Export.RetryInitial > c.Local.Export.RetryMax {
		return fmt.Errorf("local.export.retry_initial must be <= local.export.retry_max")
	}
	if c.Local.Export.MaxInflight != 1 {
		return fmt.Errorf("local.export.max_inflight must be 1")
	}
	if c.Local.Storage.MaxBytes <= 0 || c.Local.Storage.MinFreeBytes <= 0 || c.Local.Storage.SegmentSize <= 0 || c.Local.Storage.SignalMaxCount <= 0 {
		return fmt.Errorf("local.storage limits must be positive")
	}
	if c.Health.Interval <= 0 {
		return fmt.Errorf("health.interval must be positive")
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
	root, nested := "", ""
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := stripComment(scanner.Text())
		if strings.TrimSpace(raw) == "" {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		trimmed := strings.TrimSpace(raw)
		if indent == 0 && strings.HasSuffix(trimmed, ":") {
			root = strings.TrimSuffix(trimmed, ":")
			nested = ""
			continue
		}
		if root == "" {
			return Config{}, fmt.Errorf("line %d: key outside section", lineNo)
		}
		if indent == 2 && strings.HasSuffix(trimmed, ":") {
			nested = root + "." + strings.TrimSuffix(trimmed, ":")
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return Config{}, fmt.Errorf("line %d: expected key: value", lineNo)
		}
		key = strings.TrimSpace(key)
		assignSection := root
		if indent >= 4 && nested != "" {
			assignSection = nested
		} else if indent == 2 {
			nested = ""
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
		Local: LocalConfig{
			StatePath: "/var/lib/sysarmor/agent",
			Storage:   LocalStorageConfig{MaxBytes: 10 << 30, MinFreeBytes: 2 << 30, SegmentSize: 64 << 20, SignalMaxCount: 100_000},
			Export:    LocalExportConfig{RetryInitial: time.Second, RetryMax: 30 * time.Second, RequestTimeout: 10 * time.Second, MaxInflight: 1, WireCompression: "none"},
		},
		Control:   ControlConfig{SocketPath: "/run/sysarmor/agent/control.sock"},
		Runtime:   RuntimeConfig{FeatureFlags: RuntimeFeatureFlags{MatcherStrategy: "linear"}},
		Sensor:    SensorConfig{Backend: "tetragon", Mode: "managed", EventTransport: "grpc", ServerAddress: "unix:///var/run/tetragon/tetragon.sock", ProcessCacheSize: 4096, DataCacheSize: 128, EventQueueSize: 1024, RBQueueSize: "8192", ObserveOnly: true, Restart: "always", MaxRestarts: 5, RestartWindow: time.Minute},
		Telemetry: DefaultTelemetryConfig(),
		Health:    HealthConfig{Interval: 10 * time.Second},
		Policy:    PolicyConfig{Path: "/etc/sysarmor/agent/policy.json"},
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
	case "local":
		if key != "state_path" {
			return unknown(section, key)
		}
		cfg.Local.StatePath = value
	case "local.storage":
		return assignLocalStorage(&cfg.Local.Storage, key, value)
	case "local.export":
		return assignLocalExport(&cfg.Local.Export, key, value)
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
		case "max_batch_items":
			v, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("telemetry.max_batch_items: %w", err)
			}
			cfg.Telemetry.MaxBatchItems = v
		case "max_batch_bytes":
			v, err := parseByteSize(value)
			if err != nil {
				return fmt.Errorf("telemetry.max_batch_bytes: %w", err)
			}
			cfg.Telemetry.MaxBatchBytes = int(v)
		case "flush_interval":
			d, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("telemetry.flush_interval: %w", err)
			}
			cfg.Telemetry.FlushInterval = d
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
		case "path":
			cfg.Policy.Path = value
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

func parseByteSize(raw string) (int64, error) {
	units := []struct {
		suffix     string
		multiplier int64
	}{{"GiB", 1 << 30}, {"MiB", 1 << 20}, {"KiB", 1 << 10}, {"B", 1}}
	for _, unit := range units {
		if strings.HasSuffix(raw, unit.suffix) {
			value := strings.TrimSpace(strings.TrimSuffix(raw, unit.suffix))
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil || parsed <= 0 {
				return 0, fmt.Errorf("invalid byte size %q", raw)
			}
			return parsed * unit.multiplier, nil
		}
	}
	return 0, fmt.Errorf("byte size %q requires B, KiB, MiB, or GiB", raw)
}

func assignLocalStorage(storage *LocalStorageConfig, key, raw string) error {
	if key == "signal_max_count" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("local.storage.signal_max_count: %w", err)
		}
		storage.SignalMaxCount = value
		return nil
	}
	value, err := parseByteSize(raw)
	if err != nil {
		return fmt.Errorf("local.storage.%s: %w", key, err)
	}
	switch key {
	case "max_bytes":
		storage.MaxBytes = value
	case "min_free_bytes":
		storage.MinFreeBytes = value
	case "segment_size":
		storage.SegmentSize = value
	default:
		return unknown("local.storage", key)
	}
	return nil
}

func assignLocalExport(export *LocalExportConfig, key, raw string) error {
	if key == "max_inflight" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("local.export.max_inflight: %w", err)
		}
		export.MaxInflight = value
		return nil
	}
	if key == "wire_compression" {
		export.WireCompression = raw
		return nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("local.export.%s: %w", key, err)
	}
	switch key {
	case "retry_initial":
		export.RetryInitial = value
	case "retry_max":
		export.RetryMax = value
	case "request_timeout":
		export.RequestTimeout = value
	default:
		return unknown("local.export", key)
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
