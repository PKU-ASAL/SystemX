package main

import (
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

func TestEventHasMarkerOnlyForProcessExec(t *testing.T) {
	process := &eventv1.ProcessRef{
		Binary: "/bin/sh",
		Argv:   []string{"/bin/sh", "-c", ": # node sysarmor-marker-123"},
	}
	event := &eventv1.CanonicalEvent{Behavior: "process.exec", SubjectProc: process}
	if !eventHasMarker(event, "sysarmor-marker-123") {
		t.Fatal("eventHasMarker() = false, want true")
	}
	if eventHasMarker(event, "host-marker") {
		t.Fatal("eventHasMarker() matched an unrelated marker")
	}
	exitEvent := &eventv1.CanonicalEvent{Behavior: "process.exit", SubjectProc: process}
	if eventHasMarker(exitEvent, "sysarmor-marker-123") {
		t.Fatal("eventHasMarker() accepted a process.exit event")
	}
}

func TestSignalMatchesEventRefAndSeverity(t *testing.T) {
	signal := &signalv1.Signal{
		RuleId:    "web_runtime_spawns_shell",
		Severity:  "high",
		EventRefs: []string{"event-123"},
	}
	if !signalMatchesAnyEvent(signal, []string{"event-123"}) {
		t.Fatal("signalMatchesAnyEvent() = false, want true")
	}
	signal.EventRefs = []string{"event-other"}
	if signalMatchesAnyEvent(signal, []string{"event-123"}) {
		t.Fatal("signalMatchesAnyEvent() accepted an unrelated event")
	}
	signal.EventRefs = []string{"event-123"}
	signal.Severity = "medium"
	if signalMatchesAnyEvent(signal, []string{"event-123"}) {
		t.Fatal("signalMatchesAnyEvent() accepted a non-high signal")
	}
}

func TestSignalMatchesAnyMarkerEvent(t *testing.T) {
	signal := &signalv1.Signal{Severity: "high", EventRefs: []string{"event-outer"}}
	markerEvents := []string{"event-inner", "event-outer"}
	if !signalMatchesAnyEvent(signal, markerEvents) {
		t.Fatal("signalMatchesAnyEvent() = false, want true")
	}
}
