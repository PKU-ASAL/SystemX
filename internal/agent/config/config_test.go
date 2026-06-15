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
	if cfg.Sensor.EventSource != "" {
		t.Fatalf("example event_source = %q, want managed mode empty source", cfg.Sensor.EventSource)
	}
	if cfg.Sensor.TetraPath == "" || cfg.Sensor.TetragonPath == "" {
		t.Fatalf("example managed tetragon paths missing: %+v", cfg.Sensor)
	}
}

func TestSystemdUnitStartsAgentDaemon(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "deployments", "systemd", "sysarmor-agent.service"))
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
  scenario: apt-fileless-c2-managed

manager:
  address: http://10.66.0.10:9443
  transport: http

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  btf_path: /tmp/vmlinux
  bpffs_path: /tmp/bpf
  require_btf: true
  require_bpffs: true
  scope_type: container
  scope_selector: abc123
  fake_startup_events: 5
  observe_only: true
  restart: always
  max_parse_errors: 3
  max_dropped_events: 4

spool:
  path: /var/lib/sysarmor/agent/spool
  max_bytes: 268435456
  batch_size: 256
  flush_interval: 1s

upload:
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
	if cfg.Agent.ID != "node-a" || cfg.Sensor.Backend != "tetragon" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Agent.Scenario != "apt-fileless-c2-managed" {
		t.Fatalf("scenario = %q", cfg.Agent.Scenario)
	}
	if cfg.Sensor.ScopeType != "container" || cfg.Sensor.ScopeSelector != "abc123" {
		t.Fatalf("scope = %q/%q", cfg.Sensor.ScopeType, cfg.Sensor.ScopeSelector)
	}
	if cfg.Sensor.MaxParseErrors != 3 || cfg.Sensor.MaxDroppedEvents != 4 {
		t.Fatalf("parse/drop thresholds = %d/%d", cfg.Sensor.MaxParseErrors, cfg.Sensor.MaxDroppedEvents)
	}
	if cfg.Sensor.FakeStartupEvents != 5 {
		t.Fatalf("fake_startup_events = %d", cfg.Sensor.FakeStartupEvents)
	}
	if cfg.Sensor.BTFPath != "/tmp/vmlinux" || cfg.Sensor.BPFFSPath != "/tmp/bpf" || !cfg.Sensor.RequireBTF || !cfg.Sensor.RequireBPFFS {
		t.Fatalf("capability config = %+v", cfg.Sensor)
	}
	if cfg.Spool.BatchSize != 256 {
		t.Fatalf("batch size = %d", cfg.Spool.BatchSize)
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
  transport: http

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope_type: vm

spool:
  path: /var/lib/sysarmor/agent/spool

upload:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "sensor.scope_type must be one of") {
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
  transport: http

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope_type: container

spool:
  path: /var/lib/sysarmor/agent/spool

upload:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "sensor.scope_selector is required when sensor.scope_type=container") {
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
  transport: http

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope_type: host
  scope_selector: abc123

spool:
  path: /var/lib/sysarmor/agent/spool

upload:
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s

health:
  interval: 10s
`)
	_, err := LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "sensor.scope_selector must be empty when sensor.scope_type=host") {
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
  transport: http

sensor:
  backend: tetragon
  mode: managed
  policy_path: /etc/sysarmor/policies/sysarmor-tetragon.yaml
  scope_type: container
  scope_selector: abc123
  container_id_prefix: def456

spool:
  path: /var/lib/sysarmor/agent/spool

upload:
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
spool:
  path: /tmp/spool
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
