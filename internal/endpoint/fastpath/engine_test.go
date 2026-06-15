package fastpath

import (
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
)

func TestEndpointSignalsForFilelessC2(t *testing.T) {
	e := New()
	events := []*eventv1.CanonicalEvent{
		execEvent("e1", "lin-a", "p1", "parent", "/bin/bash", nil),
		connectEvent("e2", "lin-a", "p2", "/usr/bin/curl", "10.66.0.99:8080"),
		writeEvent("e3", "lin-a", "p2", "/usr/bin/curl", "/dev/shm/x.sh"),
		connectEvent("e4", "lin-a", "p3", "/bin/bash", "10.66.0.99:443"),
	}
	var names []string
	var terminal bool
	for _, ev := range events {
		for _, sig := range e.Process(ev) {
			names = append(names, sig.GetName())
			if sig.GetName() == "reverse_shell_pattern" && sig.GetTerminal() && sig.GetEvidence() != nil {
				terminal = true
			}
			if len(sig.GetEntities()) == 0 {
				t.Fatalf("signal %s has no entities", sig.GetName())
			}
		}
	}
	for _, want := range []string{"web_runtime_spawns_shell", "download_by_lolbin", "payload_dropped", "reverse_shell_pattern"} {
		if !contains(names, want) {
			t.Fatalf("missing signal %s in %v", want, names)
		}
	}
	if !terminal {
		t.Fatal("reverse_shell_pattern terminal evidence missing")
	}
}

func TestStagedDropKeepsEndpointNonTerminal(t *testing.T) {
	e := New()
	events := []*eventv1.CanonicalEvent{
		writeEvent("e1", "lin-a", "p1", "/usr/bin/curl", "/var/lib/app/plugins/helper"),
		execEvent("e2", "lin-b", "helper-stable", "", "/var/lib/app/plugins/helper", []string{"/var/lib/app/plugins/helper"}),
		connectEventWithParent("e3", "lin-b", "child-stable", "helper-stable", "/bin/bash", "10.66.0.99:443"),
	}
	var names []string
	for _, ev := range events {
		for _, sig := range e.Process(ev) {
			names = append(names, sig.GetName())
			if sig.GetTerminal() {
				t.Fatalf("staged-drop endpoint signal %s should not be terminal", sig.GetName())
			}
		}
	}
	for _, want := range []string{"payload_dropped", "suspicious_exec_connect"} {
		if !contains(names, want) {
			t.Fatalf("missing signal %s in %v", want, names)
		}
	}
}

func TestStagedPayloadDownloadDoesNotMakeReverseShellTerminal(t *testing.T) {
	e := New()
	events := []*eventv1.CanonicalEvent{
		execEvent("e1", "lin-b", "helper-stable", "", "/var/lib/app/plugins/helper", []string{"/var/lib/app/plugins/helper"}),
		connectEvent("e2", "lin-b", "curl-stable", "/usr/bin/curl", "10.66.0.99:8080"),
		connectEventWithParent("e3", "lin-b", "child-stable", "helper-stable", "/bin/bash", "10.66.0.99:443"),
	}
	for _, ev := range events {
		for _, sig := range e.Process(ev) {
			if sig.GetName() == "reverse_shell_pattern" && sig.GetTerminal() {
				t.Fatal("staged helper lineage should not terminal on reverse shell solely due to download state")
			}
		}
	}
}

func TestEndpointRuleFilterDisablesSignal(t *testing.T) {
	e := NewWithRules([]string{"download_by_lolbin"})
	var names []string
	for _, ev := range []*eventv1.CanonicalEvent{
		execEvent("e1", "lin-a", "p1", "parent", "/bin/bash", nil),
		connectEvent("e2", "lin-a", "p2", "/usr/bin/curl", "10.66.0.99:8080"),
		writeEvent("e3", "lin-a", "p2", "/usr/bin/curl", "/dev/shm/x.sh"),
	} {
		for _, sig := range e.Process(ev) {
			names = append(names, sig.GetName())
		}
	}
	if !contains(names, "download_by_lolbin") {
		t.Fatalf("expected enabled download signal in %v", names)
	}
	if contains(names, "web_runtime_spawns_shell") || contains(names, "payload_dropped") {
		t.Fatalf("disabled endpoint rules emitted signals: %v", names)
	}
}

func execEvent(id, lineage, stable, parent, bin string, argv []string) *eventv1.CanonicalEvent {
	return &eventv1.CanonicalEvent{
		Id: id, Kind: eventv1.EventKind_EVENT_KIND_EXEC, LineageId: lineage, ParentStableId: parent,
		SubjectProc: &eventv1.ProcessRef{StableId: stable, Binary: bin, Argv: argv},
	}
}

func connectEvent(id, lineage, stable, bin, dst string) *eventv1.CanonicalEvent {
	return connectEventWithParent(id, lineage, stable, "", bin, dst)
}

func connectEventWithParent(id, lineage, stable, parent, bin, dst string) *eventv1.CanonicalEvent {
	return &eventv1.CanonicalEvent{
		Id: id, Kind: eventv1.EventKind_EVENT_KIND_CONNECT, LineageId: lineage, ParentStableId: parent,
		SubjectProc: &eventv1.ProcessRef{StableId: stable, Binary: bin},
		Object:      &eventv1.ObjectRef{Kind: "socket", SocketAddr: dst},
	}
}

func writeEvent(id, lineage, stable, bin, file string) *eventv1.CanonicalEvent {
	return &eventv1.CanonicalEvent{
		Id: id, Kind: eventv1.EventKind_EVENT_KIND_WRITE, LineageId: lineage,
		SubjectProc: &eventv1.ProcessRef{StableId: stable, Binary: bin},
		Object:      &eventv1.ObjectRef{Kind: "file", FilePath: file},
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
