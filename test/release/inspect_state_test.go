package main

import (
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

func TestEventHasMarkerInArgv(t *testing.T) {
	event := &eventv1.CanonicalEvent{SubjectProc: &eventv1.ProcessRef{
		Binary: "/bin/sh",
		Argv:   []string{"/bin/sh", "-c", ": # node sysarmor-marker-123"},
	}}
	if !eventHasMarker(event, "sysarmor-marker-123") {
		t.Fatal("eventHasMarker() = false, want true")
	}
	if eventHasMarker(event, "host-marker") {
		t.Fatal("eventHasMarker() matched an unrelated marker")
	}
}

func TestSignalMatchesEventRefAndSeverity(t *testing.T) {
	signal := &signalv1.Signal{
		RuleId:    "web_runtime_spawns_shell",
		Severity:  "high",
		EventRefs: []string{"event-123"},
	}
	if !signalMatchesEvent(signal, "event-123") {
		t.Fatal("signalMatchesEvent() = false, want true")
	}
	signal.EventRefs = []string{"event-other"}
	if signalMatchesEvent(signal, "event-123") {
		t.Fatal("signalMatchesEvent() accepted an unrelated event")
	}
	signal.EventRefs = []string{"event-123"}
	signal.Severity = "medium"
	if signalMatchesEvent(signal, "event-123") {
		t.Fatal("signalMatchesEvent() accepted a non-high signal")
	}
}
