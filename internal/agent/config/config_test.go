package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryExampleConfigLoads(t *testing.T) {
	cfg, err := LoadFile(filepath.Join("..", "..", "..", "configs", "agent.example.yaml"))
	if err != nil {
		t.Fatalf("LoadFile(agent.example.yaml) error = %v", err)
	}
	if cfg.Manager.Transport != "" || cfg.Local.StatePath != "/var/lib/sysarmor/agent" {
		t.Fatalf("example is not standalone: manager=%+v agent=%+v", cfg.Manager, cfg.Agent)
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

func TestLoadFileParsesConvergedRuntimeConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
local:
  state_path: /var/lib/sysarmor/agent
  storage:
    max_bytes: 10GiB
    min_free_bytes: 2GiB
    segment_size: 64MiB
    signal_max_count: 100000
  export:
    retry_initial: 1s
    retry_max: 30s
    request_timeout: 10s
    max_inflight: 1
    wire_compression: none
telemetry:
  max_batch_items: 256
  max_batch_bytes: 256KiB
  flush_interval: 1s
sensor:
  backend: fake
policy:
  path: /etc/sysarmor/agent/policy.json
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Local.StatePath != "/var/lib/sysarmor/agent" || cfg.Local.Storage.SegmentSize != 64<<20 || cfg.Local.Export.MaxInflight != 1 {
		t.Fatalf("local config=%+v", cfg.Local)
	}
	if cfg.Telemetry.MaxBatchItems != 256 || cfg.Telemetry.MaxBatchBytes != 256<<10 || cfg.Policy.Path == "" {
		t.Fatalf("config=%+v", cfg)
	}
}

func TestLoadFileRejectsLegacyRuntimeSections(t *testing.T) {
	for name, document := range map[string]string{
		"agent state path": "agent:\n  state_path: /tmp/state\n",
		"storage":          "storage:\n  max_bytes: 1GiB\n",
		"data plane":       "data_plane:\n  retry_initial: 1s\n",
		"telemetry name":   "telemetry:\n  batch_size: 10\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent.yaml")
			write(t, path, document)
			if _, err := LoadFile(path); err == nil {
				t.Fatal("legacy config accepted")
			}
		})
	}
}

func TestLoadFileRejectsLegacyCloudIdentity(t *testing.T) {
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
  max_batch_items: 256
  max_batch_bytes: 256KiB
  flush_interval: 1s

health:
  interval: 10s
`)
	if _, err := LoadFile(path); err == nil {
		t.Fatal("legacy cloud identity accepted")
	}
}

func TestSystemdUnitStartsAgentDaemon(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deployments", "agent", "systemd", "sysarmor-agent.service"))
	if err != nil {
		t.Fatalf("ReadFile(systemd unit) error = %v", err)
	}
	unit := string(data)
	for _, want := range []string{
		"ExecStart=/opt/sysarmor/agent/bin/sysarmor-agent run --config /etc/sysarmor/agent/agent.yaml",
		"RuntimeDirectory=sysarmor/agent",
		"WorkingDirectory=/opt/sysarmor/agent",
		"Restart=always",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("systemd unit missing %q:\n%s", want, unit)
		}
	}
	if strings.Contains(unit, "network-online.target") {
		t.Fatal("standalone service depends on network-online")
	}
}

func TestInstallerUsesUnifiedLayoutAndStartsService(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deployments", "agent", "install-agent.sh"))
	if err != nil {
		t.Fatal(err)
	}
	installer := string(data)
	for _, want := range []string{
		`/etc/sysarmor/agent/agent.yaml`,
		`/etc/sysarmor/agent/policy.json`,
		`SYSARMOR_CTL_BIN`,
		`systemctl enable --now sysarmor-agent`,
		`systemctl is-active --quiet sysarmor-agent`,
	} {
		if !strings.Contains(installer, want) {
			t.Fatalf("installer missing %q", want)
		}
	}
}

func TestReleaseBuilderPackagesAgentControlTool(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deployments", "packages", "build-release.sh"))
	if err != nil {
		t.Fatal(err)
	}
	builder := string(data)
	for _, want := range []string{`CTL_BIN=`, `--ctl-bin "$CTL_BIN"`} {
		if !strings.Contains(builder, want) {
			t.Fatalf("release builder missing %q", want)
		}
	}
}

func TestStandaloneDeploymentConfigLoads(t *testing.T) {
	cfg, err := LoadFile(filepath.Join("..", "..", "..", "deployments", "agent", "standalone.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Manager.Transport != "" || cfg.Local.Storage.MaxBytes != 10<<30 || cfg.Local.StatePath == "" {
		t.Fatalf("config=%+v", cfg)
	}
}

func TestLoadFileValidatesExampleShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
agent:
  label.scenario: apt-fileless-c2-managed
  label.env: test
  label.deployment: endpoint-refinement

sensor:
  backend: tetragon
  mode: managed
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
  max_batch_items: 256
  max_batch_bytes: 256KiB
  flush_interval: 1s

health:
  interval: 10s

policy:
  path: /etc/sysarmor/agent/policy.json

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
	if cfg.Sensor.Backend != "tetragon" {
		t.Fatalf("unexpected config: %+v", cfg)
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
	if cfg.Telemetry.MaxBatchItems != 256 {
		t.Fatalf("batch size = %d", cfg.Telemetry.MaxBatchItems)
	}
	if cfg.Policy.Path == "" {
		t.Fatal("policy path is empty")
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
runtime:
  feature_flags:
    matcher_strategy: nope

sensor:
  backend: tetragon
  mode: managed

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "runtime.feature_flags.matcher_strategy") {
		t.Fatalf("LoadFile() error = %v, want matcher strategy validation error", err)
	}
}

func TestLoadFileAcceptsStandaloneWithoutCloudIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	write(t, path, `
local:
  state_path: `+filepath.Join(dir, "state")+`
  storage:
    max_bytes: 1GiB
    min_free_bytes: 128MiB
    segment_size: 8MiB
    signal_max_count: 1000
sensor:
  backend: fake
  mode: managed
policy:
  path: /tmp/policy.json
`)
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.ID != "" || cfg.Manager.Address != "" || cfg.Manager.Transport != "" {
		t.Fatalf("standalone config has cloud identity: %+v %+v", cfg.Agent, cfg.Manager)
	}
	if cfg.Local.Storage.MaxBytes != 1<<30 || cfg.Local.Storage.SignalMaxCount != 1000 {
		t.Fatalf("storage=%+v", cfg.Local.Storage)
	}
}

func TestLoadFileRejectsLegacyLocalTransport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
manager:
  transport: local
sensor:
  backend: fake
  mode: managed
`)
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "unknown section \"manager\"") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadFileRejectsLegacyFlatScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
sensor:
  backend: tetragon
  mode: managed
  scope_type: container
  scope_selector: abc123

health:
  interval: 10s
`)
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), "sensor.scope_type") {
		t.Fatalf("legacy flat scope error=%v", err)
	}
}

func TestLoadFileAcceptsCanonicalNestedScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
sensor:
  backend: tetragon
  mode: managed
  scope:
    type: pod
    selector: pod-a

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

func TestLoadFileAcceptsNamespaceSelfScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
sensor:
  backend: tetragon
  mode: managed
  scope:
    type: namespace
    selector: self

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
	if scope.Type != "namespace" || scope.Selector != "self" {
		t.Fatalf("effective scope = %q/%q", scope.Type, scope.Selector)
	}
}

func TestLoadFileRejectsNamespaceLegacySelector(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
sensor:
  backend: tetragon
  mode: managed
  scope:
    type: namespace
    selector: kubepods.slice/pod-a

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "sensor scope: namespace scope selector must be self") {
		t.Fatalf("LoadFile() error = %v, want namespace self validation error", err)
	}
}

func TestLoadFileRejectsInvalidScopeType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
sensor:
  backend: tetragon
  mode: managed
  scope:
    type: vm

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
sensor:
  backend: tetragon
  mode: managed
  scope:
    type: container

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
sensor:
  backend: tetragon
  mode: managed
  scope:
    type: host
    selector: abc123

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
sensor:
  backend: tetragon
  mode: managed
  scope_type: container
  scope_selector: abc123
  container_id_prefix: def456

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "sensor.scope_type") {
		t.Fatalf("LoadFile() error = %v", err)
	}
}

func TestLoadFileReportsMissingRequiredFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	write(t, path, `
local:
  state_path: ""
sensor:
  backend: tetragon
  mode: managed
policy:
  path: /tmp/policy.yaml
`)
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil")
	}
	for _, want := range []string{"local.state_path"} {
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
