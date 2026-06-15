package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/tamper"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/tetragon"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/transport/link1"
	"google.golang.org/protobuf/encoding/protojson"
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

func TestRunnerSpoolsConfiguredScenario(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token", Scenario: "daemon-scenario"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:9443", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := runner.Run(context.Background(), Options{Once: true}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	batch := loadOnlySpoolBatch(t, cfg.Spool.Path)
	if got := batch.GetEvents()[0].GetScenario(); got != "daemon-scenario" {
		t.Fatalf("event scenario = %q", got)
	}
	for _, sig := range batch.GetSignals() {
		if got := sig.GetScenario(); got != "daemon-scenario" {
			t.Fatalf("signal %s scenario = %q", sig.GetName(), got)
		}
	}
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
		switch r.URL.Path {
		case "/api/v1/upload":
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			closeUploaded.Do(func() {
				close(uploaded)
			})
		case "/api/v1/agent-health":
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
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

func TestRunnerBackgroundUploadLoopBacksOffAndRecovers(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var (
		mu       sync.Mutex
		attempts []time.Time
	)
	uploaded := make(chan struct{})
	var closeUploaded sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/upload":
			mu.Lock()
			attempts = append(attempts, time.Now())
			n := len(attempts)
			mu.Unlock()
			if n < 4 {
				http.Error(w, "temporary outage", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			closeUploaded.Do(func() {
				close(uploaded)
			})
		case "/api/v1/agent-health":
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &bytes.Buffer{}})
	}()
	select {
	case <-uploaded:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovered upload")
	}
	mu.Lock()
	gotAttempts := append([]time.Time(nil), attempts...)
	mu.Unlock()
	if len(gotAttempts) != 4 {
		t.Fatalf("attempt count = %d, want 4", len(gotAttempts))
	}
	if elapsed := gotAttempts[len(gotAttempts)-1].Sub(gotAttempts[0]); elapsed < 20*time.Millisecond {
		t.Fatalf("elapsed = %s, want retry backoff instead of busy loop", elapsed)
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
			t.Fatalf("spool batches after recovered drain = %v", matches)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunnerGracefulShutdownLeavesSpoolForLaterDrain(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:1", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: 5 * time.Millisecond},
		Upload:  config.UploadConfig{RetryInitial: 50 * time.Millisecond, RetryMax: 50 * time.Millisecond, RequestTimeout: 5 * time.Millisecond},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &bytes.Buffer{}})
	}()

	deadline := time.After(2 * time.Second)
	for {
		matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for spool batch before shutdown")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}

	batch := loadOnlySpoolBatch(t, cfg.Spool.Path)
	if got := batch.GetEvents()[0].GetId(); got == "" {
		t.Fatalf("spooled event id = empty")
	}
}

func TestRunnerGracefulShutdownDrainsSpoolWhenManagerAvailable(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	uploaded := make(chan analyticsv1.UploadBatch, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/upload":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read batch body: %v", err)
			}
			var batch analyticsv1.UploadBatch
			if err := protojson.Unmarshal(body, &batch); err != nil {
				t.Fatalf("decode batch: %v", err)
			}
			select {
			case uploaded <- batch:
			default:
			}
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case "/api/v1/agent-health":
			w.Header().Set("content-type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: server.URL, Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Hour},
		Upload:  config.UploadConfig{RetryInitial: 50 * time.Millisecond, RetryMax: 50 * time.Millisecond, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	var out bytes.Buffer
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &out})
	}()

	deadline := time.After(2 * time.Second)
	for {
		matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for spool batch before shutdown")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
	select {
	case batch := <-uploaded:
		if len(batch.GetEvents()) != 1 || batch.GetEvents()[0].GetId() == "" {
			t.Fatalf("uploaded batch = %+v", batch)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for shutdown drain upload; output=%q", out.String())
	}
	matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("spool batches after shutdown drain = %v", matches)
	}
	if !strings.Contains(out.String(), "agent shutdown drain: uploaded=1 remaining=0") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestUploadWorkerRecoversUnackedBatchesAfterRestart(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spoolPath := filepath.Join(dir, "spool")
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:1", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: spoolPath, MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Second},
		Upload:  config.UploadConfig{RetryInitial: time.Second, RetryMax: time.Second, RequestTimeout: 5 * time.Millisecond},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	first, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Run(context.Background(), Options{Once: true}); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(spoolPath, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("spool batches after first run = %v", matches)
	}

	oldBatch := loadOnlySpoolBatch(t, spoolPath)
	oldID := oldBatch.GetEvents()[0].GetId()
	uploadCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/upload" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read batch body: %v", err)
		}
		var batch analyticsv1.UploadBatch
		if err := protojson.Unmarshal(body, &batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		uploadCount++
		if uploadCount > 1 {
			t.Fatalf("unexpected extra upload: %+v", batch)
		}
		for _, ev := range batch.GetEvents() {
			if ev.GetId() != oldID {
				t.Fatalf("unexpected uploaded id = %q want %q", ev.GetId(), oldID)
			}
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	worker, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	worker.Config.Manager.Address = server.URL
	queue, err := spool.OpenWithLimit(spoolPath, cfg.Spool.MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	up, err := worker.uploadWorker(queue)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := up.DrainOnce(context.Background())
	if err != nil {
		t.Fatalf("DrainOnce() error = %v", err)
	}
	if stats.UploadedBatches != 1 || stats.RemainingBatches != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	matches, err = filepath.Glob(filepath.Join(spoolPath, "*.batch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("spool batches after recovery drain = %v", matches)
	}
	if uploadCount != 1 {
		t.Fatalf("uploadCount = %d", uploadCount)
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

func TestRunnerReportsFinalDegradedHealthOnShutdown(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var (
		mu       sync.Mutex
		healths  []map[string]any
	)
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
		mu.Lock()
		healths = append(healths, body)
		mu.Unlock()
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: server.URL, Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Hour},
		Upload:  config.UploadConfig{RetryInitial: 10 * time.Millisecond, RetryMax: 10 * time.Millisecond, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: 5 * time.Millisecond},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &bytes.Buffer{}})
	}()

	deadline := time.After(2 * time.Second)
	for {
		mu.Lock()
		count := len(healths)
		mu.Unlock()
		if count > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for initial health report")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(healths) == 0 {
		t.Fatal("health report count = 0, want at least 1")
	}
	last := healths[len(healths)-1]
	if last["status"] != "degraded" {
		t.Fatalf("final health status = %v, want degraded", last["status"])
	}
	sensor, ok := last["sensor_health"].(map[string]any)
	if !ok {
		t.Fatalf("final health missing sensor_health: %+v", last)
	}
	if sensor["running"] != false {
		t.Fatalf("final sensor running = %v, want false", sensor["running"])
	}
}

func TestRunnerReportsStartupFailureHealthToManager(t *testing.T) {
	reported := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-SysArmor-Agent-Token"); got != "dev-token" {
			t.Errorf("token header = %q", got)
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
	}))
	defer server.Close()
	runner := &Runner{
		Config: config.Config{
			Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
			Manager: config.ManagerConfig{Address: server.URL, Transport: "http"},
			Sensor:  config.SensorConfig{Backend: "tetragon"},
			Spool:   config.SpoolConfig{Path: filepath.Join(t.TempDir(), "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Second},
			Upload:  config.UploadConfig{RetryInitial: 10 * time.Millisecond, RetryMax: 10 * time.Millisecond, RequestTimeout: time.Second},
			Health:  config.HealthConfig{Interval: time.Hour},
		},
		Sensor: &capabilityErrorSensor{err: errors.New("btf unavailable")},
	}
	err := runner.Run(context.Background(), Options{Out: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "btf unavailable") {
		t.Fatalf("Run() error = %v", err)
	}
	select {
	case body := <-reported:
		if body["status"] != "degraded" {
			t.Fatalf("health body = %+v", body)
		}
		sensor, ok := body["sensor_health"].(map[string]any)
		if !ok {
			t.Fatalf("sensor_health missing: %+v", body)
		}
		if sensor["running"] != false {
			t.Fatalf("sensor running = %v", sensor["running"])
		}
		if got, _ := sensor["last_error"].(string); !strings.Contains(got, "probe: btf unavailable") {
			t.Fatalf("sensor last_error = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for startup failure health report")
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

func TestRunnerMarksHealthDegradedOnSpoolBackpressure(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:1", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 1, BatchSize: 10, FlushInterval: time.Hour},
		Upload:  config.UploadConfig{RetryInitial: time.Hour, RetryMax: time.Hour, RequestTimeout: 5 * time.Millisecond},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rt := sensorruntime.New(runner.Sensor)
	if _, err := rt.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rt.Apply(context.Background(), contract.CollectionIntent{ObserveOnly: true}); err != nil {
		t.Fatal(err)
	}
	queue, err := spool.OpenWithLimit(cfg.Spool.Path, cfg.Spool.MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Append(&analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", Version: "test"},
		Events: []*eventv1.CanonicalEvent{{
			Id:      "event-a",
			AgentId: "agent-a",
			HostId:  "host-a",
			Kind:    eventv1.EventKind_EVENT_KIND_EXEC,
		}},
	}); !spool.IsBackpressure(err) {
		t.Fatalf("Append() error = %v, want backpressure", err)
	}
	worker, err := runner.uploadWorker(queue)
	if err != nil {
		t.Fatal(err)
	}
	health, err := runner.collectHealth(context.Background(), rt, queue, worker, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "degraded" || health.Queue.BackpressureCount != 1 || health.Queue.DroppedBatches != 1 || health.Queue.LastError == "" {
		t.Fatalf("health = %+v", health)
	}
}

func TestRunnerSpoolsTamperSignalFromHealth(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:1", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true, RestartWindow: time.Hour},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Hour},
		Upload:  config.UploadConfig{RetryInitial: time.Hour, RetryMax: time.Hour, RequestTimeout: 5 * time.Millisecond},
		Health:  config.HealthConfig{Interval: 5 * time.Millisecond},
	}
	runner := &Runner{
		Config: cfg,
		Sensor: &healthOnlySensor{health: contract.Health{
			Backend:        "tetragon",
			Installed:      true,
			PolicyLoaded:   true,
			Running:        false,
			RestartCount:   3,
			LastExitReason: "exit status 7",
			LastError:      "exit status 7",
		}},
	}
	errCh := make(chan error, 1)
	var out bytes.Buffer
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &out})
	}()
	var batch *analyticsv1.UploadBatch
	deadline := time.After(2 * time.Second)
	for batch == nil {
		matches, err := filepath.Glob(filepath.Join(cfg.Spool.Path, "*.batch.json"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range matches {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var candidate analyticsv1.UploadBatch
			if err := protojson.Unmarshal(data, &candidate); err != nil {
				t.Fatal(err)
			}
			if len(candidate.GetSignals()) > 0 && candidate.GetSignals()[0].GetName() == tamper.SignalName {
				batch = &candidate
				break
			}
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for tamper spool batch; output=%q", out.String())
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
	sig := batch.GetSignals()[0]
	if sig.GetName() != tamper.SignalName || !sig.GetTerminal() || sig.GetWhere() != signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT {
		t.Fatalf("tamper signal = %+v", sig)
	}
}

func TestRunnerUploadsTamperSignalToManager(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	managerStore := &store.Store{}
	server := httptest.NewServer(link1.NewServerWithAuth(managerStore, "dev-token").Handler())
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: server.URL, Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true, RestartWindow: time.Hour},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: 5 * time.Millisecond},
		Upload:  config.UploadConfig{RetryInitial: 5 * time.Millisecond, RetryMax: 10 * time.Millisecond, RequestTimeout: time.Second},
		Health:  config.HealthConfig{Interval: 5 * time.Millisecond},
	}
	runner := &Runner{
		Config: cfg,
		Sensor: &healthOnlySensor{health: contract.Health{
			Backend:        "tetragon",
			Installed:      true,
			PolicyLoaded:   true,
			Running:        false,
			RestartCount:   3,
			LastExitReason: "exit status 7",
			LastError:      "exit status 7",
		}},
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx, Options{Out: &bytes.Buffer{}})
	}()
	waitForManagerSignal(t, server.URL, tamper.SignalName)
	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v", err)
	}
	if got := len(managerStore.ListSignals("agent-health", "endpoint", true)); got != 1 {
		t.Fatalf("manager tamper signal count = %d, want 1", got)
	}
}

func TestRunnerMarksHealthDegradedWhenParseThresholdExceeded(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Agent:   config.AgentConfig{ID: "agent-a", HostID: "host-a", TenantID: "default", Token: "dev-token"},
		Manager: config.ManagerConfig{Address: "http://127.0.0.1:1", Transport: "http"},
		Sensor:  config.SensorConfig{Backend: "fake", Mode: "managed", PolicyPath: policyPath, ObserveOnly: true, MaxParseErrors: 1, RestartWindow: time.Hour},
		Spool:   config.SpoolConfig{Path: filepath.Join(dir, "spool"), MaxBytes: 4096, BatchSize: 10, FlushInterval: time.Hour},
		Upload:  config.UploadConfig{RetryInitial: time.Hour, RetryMax: time.Hour, RequestTimeout: 5 * time.Millisecond},
		Health:  config.HealthConfig{Interval: time.Hour},
	}
	runner := &Runner{
		Config: cfg,
		Sensor: &healthOnlySensor{health: contract.Health{
			Backend:      "tetragon",
			Installed:    true,
			PolicyLoaded: true,
			Running:      true,
			ParseErrors:  2,
		}},
	}
	rt := sensorruntime.New(runner.Sensor)
	if _, err := rt.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rt.Apply(context.Background(), contract.CollectionIntent{ObserveOnly: true}); err != nil {
		t.Fatal(err)
	}
	queue, err := spool.OpenWithLimit(cfg.Spool.Path, cfg.Spool.MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := runner.uploadWorker(queue)
	if err != nil {
		t.Fatal(err)
	}
	health, err := runner.collectHealth(context.Background(), rt, queue, worker, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "degraded" || health.Sensor.ParseErrors != 2 {
		t.Fatalf("health = %+v", health)
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

func TestTetragonRestartPolicyFromConfig(t *testing.T) {
	policy, err := tetragonRestartPolicy(config.SensorConfig{
		Restart:       "always",
		MaxRestarts:   7,
		RestartWindow: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("tetragonRestartPolicy() error = %v", err)
	}
	if policy != (tetragon.ProcessRestartPolicy{Enabled: true, MaxRestarts: 7, Delay: 25 * time.Millisecond}) {
		t.Fatalf("policy = %+v", policy)
	}
	disabled, err := tetragonRestartPolicy(config.SensorConfig{Restart: "never"})
	if err != nil {
		t.Fatalf("tetragonRestartPolicy(never) error = %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("disabled policy = %+v", disabled)
	}
	if _, err := tetragonRestartPolicy(config.SensorConfig{Restart: "sometimes"}); err == nil {
		t.Fatal("tetragonRestartPolicy(unknown) error = nil")
	}
}

type healthOnlySensor struct {
	health contract.Health
}

type capabilityErrorSensor struct {
	err error
}

func (s *capabilityErrorSensor) Capability(context.Context) (contract.Capability, error) {
	return contract.Capability{}, s.err
}

func (s *capabilityErrorSensor) Apply(context.Context, contract.CollectionIntent) error {
	return nil
}

func (s *capabilityErrorSensor) Subscribe(context.Context, contract.CollectionIntent) (<-chan contract.EventEnvelope, error) {
	return nil, nil
}

func (s *capabilityErrorSensor) Enforce(_ context.Context, cmd contract.EnforcementCmd) (contract.EnforcementAck, error) {
	return contract.UnsupportedAck(cmd, "test sensor"), nil
}

func (s *capabilityErrorSensor) Health(context.Context) (contract.Health, error) {
	return contract.Health{}, nil
}

func (s *healthOnlySensor) Capability(context.Context) (contract.Capability, error) {
	return contract.Capability{Backend: s.health.Backend, Version: "test", SupportsHealth: true}, nil
}

func (s *healthOnlySensor) Apply(context.Context, contract.CollectionIntent) error {
	return nil
}

func (s *healthOnlySensor) Subscribe(ctx context.Context, _ contract.CollectionIntent) (<-chan contract.EventEnvelope, error) {
	out := make(chan contract.EventEnvelope)
	go func() {
		defer close(out)
		<-ctx.Done()
	}()
	return out, nil
}

func (s *healthOnlySensor) Enforce(_ context.Context, cmd contract.EnforcementCmd) (contract.EnforcementAck, error) {
	return contract.UnsupportedAck(cmd, "test sensor"), nil
}

func (s *healthOnlySensor) Health(context.Context) (contract.Health, error) {
	return s.health, nil
}

func waitForManagerSignal(t *testing.T, managerURL, name string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		resp, err := http.Get(managerURL + "/api/v1/signals?scenario=agent-health&layer=endpoint&terminal=true")
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if resp.StatusCode == http.StatusOK && strings.Contains(string(body), name) {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for manager signal %q", name)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func assertSpoolBatch(t *testing.T, dir string) {
	t.Helper()
	batch := loadOnlySpoolBatch(t, dir)
	if batch.GetAgent().GetAgentId() != "agent-a" {
		t.Fatalf("spool batch agent = %+v", batch.GetAgent())
	}
}

func loadOnlySpoolBatch(t *testing.T, dir string) *analyticsv1.UploadBatch {
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
	var batch analyticsv1.UploadBatch
	if err := protojson.Unmarshal(data, &batch); err != nil {
		t.Fatalf("decode spool batch: %v\n%s", err, string(data))
	}
	return &batch
}
