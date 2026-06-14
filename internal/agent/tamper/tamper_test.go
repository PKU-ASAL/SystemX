package tamper

import (
	"testing"
	"time"

	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
)

func TestDetectorEmitsSensorTamperSignal(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	detector := &Detector{}
	health := agenthealth.AgentHealth{
		AgentID:       "agent-a",
		HostID:        "host-a",
		TenantID:      "default",
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
	if got := detector.Evaluate(health, now, DefaultOptions()); got != nil {
		t.Fatalf("duplicate Evaluate() = %+v", got)
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
