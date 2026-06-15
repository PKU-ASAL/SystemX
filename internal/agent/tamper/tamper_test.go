package tamper

import (
	"strings"
	"testing"
	"time"

	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
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
	if got := detector.Evaluate(health, now, DefaultOptions()); got != nil {
		t.Fatalf("duplicate Evaluate() = %+v", got)
	}
	health.Scope.Selector = "def456"
	if got := detector.Evaluate(health, now, DefaultOptions()); got == nil {
		t.Fatal("Evaluate() for different scope = nil")
	}
}

func TestReasonDetectsBlindEventStream(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	health := agenthealth.AgentHealth{
		UptimeSeconds: 120,
		Sensor: agenthealth.SensorHealth{
			Backend:      "tetragon",
			PolicyLoaded: true,
			Running:      true,
		},
	}
	if got := Reason(health, now, Options{NoEventGracePeriod: time.Minute}); got != "event_stream_blind:no_events_seen" {
		t.Fatalf("Reason() = %q", got)
	}

	health.Sensor.LastEventAt = now.Add(-2 * time.Minute)
	if got := Reason(health, now, Options{NoEventGracePeriod: time.Minute}); got == "" {
		t.Fatal("Reason() for stale last_event_at = empty")
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
	if got := Reason(health, time.Now(), Options{MaxParseErrors: 2}); got != "parse_errors_exceeded:3>2" {
		t.Fatalf("parse Reason() = %q", got)
	}
	health.Sensor.ParseErrors = 0
	if got := Reason(health, time.Now(), Options{MaxDroppedEvents: 2}); got != "events_dropped_exceeded:4>2" {
		t.Fatalf("drop Reason() = %q", got)
	}
}
