package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFileValidatesExampleShape(t *testing.T) {
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
  observe_only: true
  restart: always

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
	if cfg.Spool.BatchSize != 256 {
		t.Fatalf("batch size = %d", cfg.Spool.BatchSize)
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
