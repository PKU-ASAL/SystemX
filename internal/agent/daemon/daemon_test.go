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
	assertSpoolBatch(t, cfg.Spool.Path)
}

func TestRunnerOnceWithTetragonJSONLSource(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	eventPath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC, CONNECT]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	if err := os.WriteFile(eventPath, []byte(raw+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:9443", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "tetragon", Mode: "managed", Version: "test", PolicyPath: policyPath, EventSource: eventPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1024, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
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
	for _, want := range []string{"sensor=tetragon", "agent daemon event"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output %q does not contain %q", got, want)
		}
	}
	assertSpoolBatch(t, cfg.Spool.Path)
}

func TestRunnerTetragonRequiresEventSource(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:9443", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "tetragon", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1024, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	err = runner.Run(context.Background(), Options{Once: true})
	if err == nil {
		t.Fatal("Run() error = nil")
	}
}

func assertSpoolBatch(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("spool batches = %v", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "agent-a") {
		t.Fatalf("spool batch does not contain agent identity: %s", string(data))
	}
}
