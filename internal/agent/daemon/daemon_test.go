package daemon

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
)

func TestRunnerOnceWithFakeSensor(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC, CONNECT]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:9443", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1024, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var out bytes.Buffer
	if err := runner.Run(context.Background(), Options{Once: true, Out: &out}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"agent daemon started", "sensor=fake", "agent daemon event"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
}

func TestNewRejectsTetragonUntilManagedBackendExists(t *testing.T) {
	_, err := New(config.Config{Sensor: config.SensorConfig{Backend: "tetragon"}})
	if err == nil {
		t.Fatal("New() error = nil")
	}
}
