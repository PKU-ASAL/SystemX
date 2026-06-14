package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestRunnerDrainOnceUploadsAndAcksSpool(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var uploads int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/upload" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		uploads++
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: server.URL, Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1024, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runner.Run(context.Background(), Options{Once: true, DrainOnce: true, Out: &out}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if uploads != 1 {
		t.Fatalf("uploads = %d", uploads)
	}
	matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("spool batches after drain = %v", matches)
	}
	if !strings.Contains(out.String(), "agent upload drain: uploaded=1 remaining=0") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunnerBackgroundUploadLoopDrainsSpool(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uploaded := make(chan struct{})
	var closeUploaded sync.Once
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/upload" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		closeUploaded.Do(func() {
			close(uploaded)
		})
	}))
	defer server.Close()
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: server.URL, Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: 5 * time.Millisecond},
		Upload:  config.UploadConfig{RetryInitial: 5 * time.Millisecond, RetryMax: 10 * time.Millisecond, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &bytes.Buffer{}})
	}()
	select {
	case <-uploaded:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for background upload")
	}
	deadline := time.After(2 * time.Second)
	for {
		matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("spool batches after background drain = %v", matches)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunnerReportsHealthToManager(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reported := make(chan map[string]any, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-SysArmor-Agent-Token"); got != "dev-token" {
			t.Errorf("token header = %q", got)
		}
		if r.URL.Path == "/api/v1/upload" {
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			return
		}
		if r.URL.Path != "/api/v1/agent-health" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode health: %v", err)
		}
		select {
		case reported <- body:
		default:
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		cancel()
	}))
	defer server.Close()
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: server.URL, Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Hour},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: 5 * time.Millisecond},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &bytes.Buffer{}})
	}()
	select {
	case body := <-reported:
		if body["agent_id"] != "agent-a" || body["tenant_id"] != "default" {
			t.Fatalf("health body = %+v", body)
		}
		if _, ok := body["sensor_health"].(map[string]any); !ok {
			t.Fatalf("health body missing sensor_health: %+v", body)
		}
		if _, ok := body["queue_health"].(map[string]any); !ok {
			t.Fatalf("health body missing queue_health: %+v", body)
		}
		if _, ok := body["upload_health"].(map[string]any); !ok {
			t.Fatalf("health body missing upload_health: %+v", body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for health report")
	}
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunnerReportsSpoolBackpressure(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:9443", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runner.Run(context.Background(), Options{Once: true, Out: &out}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(out.String(), "agent spool backpressure") {
		t.Fatalf("output = %q", out.String())
	}
	matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("spool batches = %v", matches)
	}
}

func TestNewBatchUploaderAcceptsConfiguredTimeout(t *testing.T) {
	for _, tc := range []struct {
		name      string
		transport string
	}{
		{name: "http", transport: "http"},
		{name: "grpc", transport: "grpc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up, err := newBatchUploader("127.0.0.1:9443", tc.transport, 250*time.Millisecond, "dev-token")
			if err != nil {
				t.Fatalf("newBatchUploader() error = %v", err)
			}
			if up == nil {
				t.Fatal("newBatchUploader() = nil")
			}
		})
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
