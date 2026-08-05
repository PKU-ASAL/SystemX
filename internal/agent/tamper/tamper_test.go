package tamper

import (
	"strings"
	"testing"
	"time"

	agenthealth "github.com/sysarmor/sysarmor-next-project/packages/contracts/health"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
)

func TestDetectorEmitsSensorTamperSignal(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	detector := &Detector{}
	health := agenthealth.AgentHealth{
		AgentID:       "agent-a",
		HostID:        "host-a",
		TenantID:      "default",
		Scope:         agenthealth.RuntimeScope{Type: "container", Selector: "abc123"},
		UptimeSeconds: 10,
		ObservedAt:    now,
		Sensor: agenthealth.SensorHealth{
			Backend:        "tetragon",
			PolicyLoaded:   true,
			Running:        false,
			EventsSeen:     11,
			EventsDropped:  3,
			ParseErrors:    2,
			RestartCount:   4,
			LastExitReason: "exit status 7",
		},
	}
	sig := detector.Evaluate(health, now, DefaultOptions())
	if sig == nil {
		t.Fatal("Evaluate() = nil")
	}
	if sig.GetName() != SignalName || !sig.GetTerminal() || sig.GetBaseRisk() != 90 {
		t.Fatalf("signal = %+v", sig)
	}
	if sig.GetEvidence() == nil || sig.GetEvidence().GetSummary() == "" {
		t.Fatalf("signal evidence = %+v", sig.GetEvidence())
	}
	if !hasEntity(sig.GetEntities(), "scope", "scope:container:abc123") {
		t.Fatalf("signal entities missing scope: %+v", sig.GetEntities())
	}
	if !hasEntity(sig.GetEvidence().GetEntities(), "scope", "scope:container:abc123") || !contains(sig.GetEvidence().GetSummary(), "scope=container:abc123") {
		t.Fatalf("signal evidence missing scope: %+v", sig.GetEvidence())
	}
	for _, want := range []string{"restarts=4", "parse_errors=2", "dropped_events=3", "events_seen=11"} {
		if !contains(sig.GetEvidence().GetSummary(), want) {
			t.Fatalf("signal evidence summary missing %q: %s", want, sig.GetEvidence().GetSummary())
		}
	}
	if got := detector.Evaluate(health, now, DefaultOptions()); got != nil {
		t.Fatalf("duplicate Evaluate() = %+v", got)
	}
	health.Scope.Selector = "def456"
	if got := detector.Evaluate(health, now, DefaultOptions()); got == nil {
		t.Fatal("Evaluate() for different scope = nil")
	}
}

func TestReasonDoesNotTreatQuietEventStreamAsBlind(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	health := agenthealth.AgentHealth{
		UptimeSeconds: 3600,
		Sensor: agenthealth.SensorHealth{
			Backend:      "tetragon",
			PolicyLoaded: true,
			Running:      true,
		},
	}
	if got := Reason(health, DefaultOptions()); got != "" {
		t.Fatalf("Reason() = %q, want quiet healthy sensor", got)
	}

	health.Sensor.LastEventAt = now.Add(-time.Hour)
	if got := Reason(health, DefaultOptions()); got != "" {
		t.Fatalf("Reason() with stale business event = %q, want quiet healthy sensor", got)
	}
}

func TestDetectorEmitsAgainAfterRecovery(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	detector := &Detector{}
	health := agenthealth.AgentHealth{
		AgentID: "agent-a", HostID: "host-a", TenantID: "default",
		Sensor: agenthealth.SensorHealth{Backend: "tetragon", PolicyLoaded: true, Running: false},
	}
	first := detector.Evaluate(health, now, DefaultOptions())
	if first == nil {
		t.Fatal("first failure signal = nil")
	}
	if got := detector.Evaluate(health, now.Add(time.Second), DefaultOptions()); got != nil {
		t.Fatalf("duplicate failure signal = %+v", got)
	}
	health.Sensor.Running = true
	if got := detector.Evaluate(health, now.Add(2*time.Second), DefaultOptions()); got != nil {
		t.Fatalf("recovery signal = %+v", got)
	}
	health.Sensor.Running = false
	second := detector.Evaluate(health, now.Add(3*time.Second), DefaultOptions())
	if second == nil {
		t.Fatal("failure after recovery signal = nil")
	}
	if second.GetId() == first.GetId() {
		t.Fatalf("failure after recovery reused signal ID %q", second.GetId())
	}
}

func TestDetectorDoesNotRepeatWhenThresholdCountIncreases(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	detector := &Detector{}
	health := agenthealth.AgentHealth{
		AgentID: "agent-a", HostID: "host-a", TenantID: "default",
		Sensor: agenthealth.SensorHealth{
			Backend:      "tetragon",
			PolicyLoaded: true,
			Running:      true,
			ParseErrors:  3,
		},
	}
	opts := Options{MaxParseErrors: 2}
	if got := detector.Evaluate(health, now, opts); got == nil {
		t.Fatal("first threshold signal = nil")
	}
	health.Sensor.ParseErrors = 4
	if got := detector.Evaluate(health, now.Add(time.Second), opts); got != nil {
		t.Fatalf("increased count repeated signal = %+v", got)
	}
}

func TestDetectorEmitsWhenSensorErrorChanges(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	detector := &Detector{}
	health := agenthealth.AgentHealth{
		AgentID: "agent-a", HostID: "host-a", TenantID: "default",
		Sensor: agenthealth.SensorHealth{
			Backend:      "tetragon",
			PolicyLoaded: true,
			Running:      true,
			LastError:    "btf unavailable",
		},
	}
	if got := detector.Evaluate(health, now, DefaultOptions()); got == nil {
		t.Fatal("first sensor error signal = nil")
	}
	health.Sensor.LastError = "bpffs unavailable"
	if got := detector.Evaluate(health, now.Add(time.Second), DefaultOptions()); got == nil {
		t.Fatal("changed sensor error signal = nil")
	}
}

func hasEntity(entities []*signalv1.EntityRef, kind, key string) bool {
	for _, entity := range entities {
		if entity.GetKind() == kind && entity.GetKey() == key {
			return true
		}
	}
	return false
}

func contains(value, want string) bool {
	return strings.Contains(value, want)
}

func TestReasonDetectsParseAndDropThresholds(t *testing.T) {
	health := agenthealth.AgentHealth{
		Sensor: agenthealth.SensorHealth{
			Backend:       "tetragon",
			PolicyLoaded:  true,
			Running:       true,
			ParseErrors:   3,
			EventsDropped: 4,
		},
	}
	if got := Reason(health, Options{MaxParseErrors: 2}); got != "parse_errors_exceeded:3>2" {
		t.Fatalf("parse Reason() = %q", got)
	}
	health.Sensor.ParseErrors = 0
	if got := Reason(health, Options{MaxDroppedEvents: 2}); got != "events_dropped_exceeded:4>2" {
		t.Fatalf("drop Reason() = %q", got)
	}
}
