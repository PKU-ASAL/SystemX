package normalize

import (
	"testing"

	sensorv1 "github.com/sysarmor/sysarmor-next-project/api/proto/sensor/v1"
)

func TestNormalizeInheritsLineageFromParent(t *testing.T) {
	n := New("agent-a", "host-a", nil)
	parent := n.Normalize(&sensorv1.SensorEvent{
		Behavior: "process.exec",
		Proc:     &sensorv1.RawProcess{Pid: 100, Binary: "/usr/bin/java", StartTimeNs: 10},
	})
	child := n.Normalize(&sensorv1.SensorEvent{
		Behavior: "process.exec",
		Proc:     &sensorv1.RawProcess{Pid: 101, Ppid: 100, Binary: "/bin/bash", StartTimeNs: 20},
	})

	if parent.GetLineageId() == "" {
		t.Fatal("parent lineage is empty")
	}
	if child.GetLineageId() != parent.GetLineageId() {
		t.Fatalf("child lineage = %q, want parent lineage %q", child.GetLineageId(), parent.GetLineageId())
	}
	if child.GetParentStableId() != parent.GetSubjectProc().GetStableId() {
		t.Fatalf("child parent stable id = %q, want %q", child.GetParentStableId(), parent.GetSubjectProc().GetStableId())
	}
}

func TestStableIDChangesAcrossStartTime(t *testing.T) {
	a := StableID("host-a", 42, 1)
	b := StableID("host-a", 42, 2)
	if a == b {
		t.Fatal("stable id should change when process start time changes")
	}
}

func TestNormalizeUsesSensorExecIDForParentage(t *testing.T) {
	n := New("agent-a", "host-a", nil)
	root := n.Normalize(&sensorv1.SensorEvent{
		Behavior: "process.exec",
		Proc:     &sensorv1.RawProcess{Pid: 200, Binary: "/bin/bash", SensorExecId: "exec-root"},
	})
	helper := n.Normalize(&sensorv1.SensorEvent{
		Behavior: "process.exec",
		Proc:     &sensorv1.RawProcess{Pid: 300, Ppid: 200, Binary: "/var/lib/app/plugins/helper", SensorExecId: "exec-helper", SensorParentExecId: "exec-root"},
	})
	bashAfterExec := n.Normalize(&sensorv1.SensorEvent{
		Behavior: "process.exec",
		Proc:     &sensorv1.RawProcess{Pid: 300, Ppid: 300, Binary: "/bin/bash", SensorExecId: "exec-bash", SensorParentExecId: "exec-helper"},
	})

	if helper.GetLineageId() != root.GetLineageId() {
		t.Fatalf("helper lineage = %q, want root lineage %q", helper.GetLineageId(), root.GetLineageId())
	}
	if bashAfterExec.GetParentStableId() != helper.GetSubjectProc().GetStableId() {
		t.Fatalf("bash parent stable id = %q, want helper stable id %q", bashAfterExec.GetParentStableId(), helper.GetSubjectProc().GetStableId())
	}
	if bashAfterExec.GetSubjectProc().GetStableId() == helper.GetSubjectProc().GetStableId() {
		t.Fatal("same PID with different sensor exec id should get a different stable id")
	}
}

func TestNormalizeAddsProvenanceTags(t *testing.T) {
	n := NewWithOptions("agent-a", "host-a", nil, Options{
		TenantID:      "tenant-a",
		ScopeType:     "container",
		ScopeSelector: "container-123",
		Labels:        map[string]string{"env": "test", "benchmark_run": "run-a"},
	})
	ev := n.Normalize(&sensorv1.SensorEvent{
		MonoNs:      12345,
		Behavior:    "network.connect",
		ContainerId: "container-123",
		Proc:        &sensorv1.RawProcess{Pid: 100, Binary: "/bin/bash", StartTimeNs: 10, Cgroup: "cg-a"},
		Object:      &sensorv1.RawObject{Dst: "10.0.0.1:443"},
		RawRef:      "raw-a",
	})

	if ev.GetTenantId() != "tenant-a" || ev.GetScope().GetType() != "container" || ev.GetScope().GetSelector() != "container-123" {
		t.Fatalf("provenance scope tags not set: %+v", ev)
	}
	if ev.GetContainerId() != "container-123" || ev.GetCgroup() != "cg-a" || ev.GetOccurredAtNs() != 12345 {
		t.Fatalf("runtime tags not set: %+v", ev)
	}
	if ev.GetLabels()["env"] != "test" || ev.GetLabels()["benchmark_run"] != "run-a" {
		t.Fatalf("labels not set: %+v", ev.GetLabels())
	}
}
