package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRepositoryExampleConfigLoads(t *testing.T) {
	cfg, err := LoadFile(filepath.Join("..", "..", "..", "configs", "agent.example.yaml"))
	if err != nil {
		t.Fatalf("LoadFile(agent.example.yaml) error = %v", err)
	}
	if cfg.Manager.Transport != "grpc" {
		t.Fatalf("example manager transport = %q, want grpc", cfg.Manager.Transport)
	}
	if cfg.Sensor.EventSource != "" {
		t.Fatalf("example event_source = %q, want managed mode empty source", cfg.Sensor.EventSource)
	}
	if cfg.Sensor.TetraPath == "" || cfg.Sensor.TetragonPath == "" {
		t.Fatalf("example managed tetragon paths missing: %+v", cfg.Sensor)
	}
	if cfg.Runtime.FeatureFlags.MatcherStrategy != "linear" {
		t.Fatalf("example matcher strategy = %q, want linear", cfg.Runtime.FeatureFlags.MatcherStrategy)
	}
}

func TestDefaultManagerTransportIsGRPC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token

manager:
  address: 127.0.0.1:9443

sensor:
  backend: fake
  mode: managed
  policy_path: test/policies/collection.yaml

telemetry:
  batch_size: 256
  flush_interval: 1s

data_plane:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Manager.Transport != "grpc" {
		t.Fatalf("default manager transport = %q, want grpc", cfg.Manager.Transport)
	}
}

func TestSystemdUnitStartsAgentDaemon(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deployments", "agent", "systemd", "sysarmor-agent.service"))
	if err != nil {
		t.Fatalf("ReadFile(systemd unit) error = %v", err)
	}
	unit := string(data)
	for _, want := range []string{
		"ExecStart=/usr/local/bin/sysarmor-agent run --config /etc/sysarmor/agent.yaml",
		"Restart=always",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("systemd unit missing %q:\n%s", want, unit)
		}
	}
}

func TestLoadFileValidatesExampleShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token
  label.scenario: apt-fileless-c2-managed
  label.env: test
  label.deployment: endpoint-refinement

manager:
  address: http://10.66.0.10:9443
  transport: grpc
  tls_ca: /etc/sysarmor/pki/ca.pem
  tls_cert: /etc/sysarmor/pki/agent.pem
  tls_key: /etc/sysarmor/pki/agent-key.pem
  tls_server_name: manager.sysarmor.local
  tls_insecure: false

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  btf_path: /tmp/vmlinux
  bpffs_path: /tmp/bpf
  require_btf: true
  require_bpffs: true
  scope:
    type: container
    selector: abc123
  fake_startup_events: 5
  observe_only: true
  restart: always
  max_parse_errors: 3
  max_dropped_events: 4

telemetry:
  batch_size: 256
  flush_interval: 1s

data_plane:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s

policy:
  refresh_interval: 15s

resource:
  max_active_cep_groups: 32
  max_event_refs_per_signal: 8

runtime:
  feature_flags:
    matcher_strategy: optimized
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Agent.ID != "node-a" || cfg.Sensor.Backend != "tetragon" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Manager.TLSCA != "/etc/sysarmor/pki/ca.pem" || cfg.Manager.TLSCert == "" || cfg.Manager.TLSKey == "" || cfg.Manager.TLSServerName != "manager.sysarmor.local" || cfg.Manager.TLSInsecure {
		t.Fatalf("manager TLS config = %+v", cfg.Manager)
	}
	if cfg.Agent.Labels["env"] != "test" || cfg.Agent.Labels["deployment"] != "endpoint-refinement" || cfg.Agent.Labels["scenario"] != "apt-fileless-c2-managed" {
		t.Fatalf("agent labels = %+v", cfg.Agent.Labels)
	}
	if cfg.Sensor.Scope.Type != "container" || cfg.Sensor.Scope.Selector != "abc123" {
		t.Fatalf("canonical scope = %q/%q", cfg.Sensor.Scope.Type, cfg.Sensor.Scope.Selector)
	}
	scope, err := cfg.Sensor.EffectiveScope()
	if err != nil {
		t.Fatalf("EffectiveScope() error = %v", err)
	}
	if scope.Type != "container" || scope.Selector != "abc123" {
		t.Fatalf("effective scope = %q/%q", scope.Type, scope.Selector)
	}
	if cfg.Sensor.MaxParseErrors != 3 || cfg.Sensor.MaxDroppedEvents != 4 {
		t.Fatalf("parse/drop thresholds = %d/%d", cfg.Sensor.MaxParseErrors, cfg.Sensor.MaxDroppedEvents)
	}
	if cfg.Sensor.FakeStartupEvents != 5 {
		t.Fatalf("fake_startup_events = %d", cfg.Sensor.FakeStartupEvents)
	}
	if !cfg.Sensor.ObserveOnly || cfg.Sensor.Restart != "always" {
		t.Fatalf("post-scope sensor fields not parsed: %+v", cfg.Sensor)
	}
	if cfg.Sensor.BTFPath != "/tmp/vmlinux" || cfg.Sensor.BPFFSPath != "/tmp/bpf" || !cfg.Sensor.RequireBTF || !cfg.Sensor.RequireBPFFS {
		t.Fatalf("capability config = %+v", cfg.Sensor)
	}
	if cfg.Telemetry.BatchSize != 256 {
		t.Fatalf("batch size = %d", cfg.Telemetry.BatchSize)
	}
	if cfg.Policy.RefreshInterval != 15*time.Second {
		t.Fatalf("policy refresh interval = %s", cfg.Policy.RefreshInterval)
	}
	if cfg.Resource.MaxActiveCEPGroups != 32 || cfg.Resource.MaxEventRefsPerSignal != 8 {
		t.Fatalf("resource config = %+v", cfg.Resource)
	}
	if cfg.Runtime.FeatureFlags.MatcherStrategy != "optimized" {
		t.Fatalf("runtime feature flags = %+v", cfg.Runtime.FeatureFlags)
	}
}

func TestLoadFileRejectsInvalidRuntimeFeatureFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token

manager:
  address: http://10.66.0.10:9443
  transport: grpc

runtime:
  feature_flags:
    matcher_strategy: nope

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml

telemetry:

data_plane:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "runtime.feature_flags.matcher_strategy") {
		t.Fatalf("LoadFile() error = %v, want matcher strategy validation error", err)
	}
}

func TestLoadFileAcceptsLegacyFlatScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token

manager:
  address: http://10.66.0.10:9443
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope_type: container
  scope_selector: abc123

telemetry:

data_plane:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if cfg.Sensor.ScopeType != "container" || cfg.Sensor.ScopeSelector != "abc123" {
		t.Fatalf("scope = %q/%q", cfg.Sensor.ScopeType, cfg.Sensor.ScopeSelector)
	}
	scope, err := cfg.Sensor.EffectiveScope()
	if err != nil {
		t.Fatalf("EffectiveScope() error = %v", err)
	}
	if scope.Type != "container" || scope.Selector != "abc123" {
		t.Fatalf("effective scope = %q/%q", scope.Type, scope.Selector)
	}
}

func TestLoadFileAcceptsCanonicalNestedScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token

manager:
  address: http://10.66.0.10:9443
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope:
    type: pod
    selector: pod-a

telemetry:

data_plane:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	scope, err := cfg.Sensor.EffectiveScope()
	if err != nil {
		t.Fatalf("EffectiveScope() error = %v", err)
	}
	if scope.Type != "pod" || scope.Selector != "pod-a" {
		t.Fatalf("effective scope = %q/%q", scope.Type, scope.Selector)
	}
}

func TestLoadFileRejectsInvalidScopeType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token

manager:
  address: http://10.66.0.10:9443
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope_type: vm

telemetry:

data_plane:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "sensor scope: scope type must be one of") {
		t.Fatalf("LoadFile() error = %v", err)
	}
}

func TestLoadFileRejectsMissingSelectorForNonHostScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token

manager:
  address: http://10.66.0.10:9443
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope_type: container

telemetry:

data_plane:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "sensor scope: scope selector is required when scope type is container") {
		t.Fatalf("LoadFile() error = %v", err)
	}
}

func TestLoadFileRejectsSelectorForHostScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token

manager:
  address: http://10.66.0.10:9443
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope_type: host
  scope_selector: abc123

telemetry:

data_plane:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "sensor scope: scope selector must be empty when scope type is host") {
		t.Fatalf("LoadFile() error = %v", err)
	}
}

func TestLoadFileRejectsConflictingLegacyContainerPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
  host_id: node-a
  tenant_id: default
  token: dev-token

manager:
  address: http://10.66.0.10:9443
  transport: grpc

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope_type: container
  scope_selector: abc123
  container_id_prefix: def456

telemetry:

data_plane:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "sensor.scope_selector conflicts with sensor.container_id_prefix") {
		t.Fatalf("LoadFile() error = %v", err)
	}
}

func TestLoadFileReportsMissingRequiredFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  id: node-a
manager:
  address: http://127.0.0.1:9443
sensor:
  backend: tetragon
  mode: managed
  policy_path: /tmp/policy.yaml
telemetry:
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil")
	}
	for _, want := range []string{"agent.host_id", "agent.tenant_id", "agent.token"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
