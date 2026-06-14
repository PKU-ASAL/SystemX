package store

import (
	"path/filepath"
	"testing"
	"time"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
)

func TestListSignalsFiltersScenarioLayerAndTerminal(t *testing.T) {
	st := &Store{}
	st.AddSignal(&signalv1.Signal{
		Id:       "s1",
		Name:     "reverse_shell_pattern",
		Scenario: "apt-fileless-c2",
		Where:    signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		Terminal: true,
	})
	st.AddSignal(&signalv1.Signal{
		Id:       "s2",
		Name:     "web_shell_chain",
		Scenario: "apt-fileless-c2",
		Where:    signalv1.SignalWhere_SIGNAL_WHERE_CLOUD,
	})
	st.AddSignal(&signalv1.Signal{
		Id:       "s3",
		Name:     "payload_dropped",
		Scenario: "apt-staged-drop",
		Where:    signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
	})

	got := st.ListSignals("apt-fileless-c2", "endpoint", true)
	if len(got) != 1 || got[0].GetId() != "s1" {
		t.Fatalf("expected terminal endpoint signal s1, got %#v", got)
	}

	got = st.ListSignals("apt-fileless-c2", "cloud", false)
	if len(got) != 1 || got[0].GetId() != "s2" {
		t.Fatalf("expected cloud signal s2, got %#v", got)
	}
}

func TestMetricsSnapshotAndReset(t *testing.T) {
	st := &Store{}
	st.RecordUpload(2, 3, 1, 1, 12*time.Millisecond)
	st.RecordUpload(1, 1, 0, 0, 4*time.Millisecond)

	got := st.MetricsSnapshot()
	if got.UploadBatches != 2 {
		t.Fatalf("upload batches = %d, want 2", got.UploadBatches)
	}
	if got.EventsIngested != 3 {
		t.Fatalf("events ingested = %d, want 3", got.EventsIngested)
	}
	if got.SignalsEmitted != 5 {
		t.Fatalf("signals emitted = %d, want 5", got.SignalsEmitted)
	}
	if got.LastConvergenceLatencyMs != 4 || got.MaxConvergenceLatencyMs != 12 {
		t.Fatalf("latency metrics = last %d max %d, want last 4 max 12", got.LastConvergenceLatencyMs, got.MaxConvergenceLatencyMs)
	}
	if got.AverageConvergenceLatency != 8 {
		t.Fatalf("average latency = %f, want 8", got.AverageConvergenceLatency)
	}

	st.DeleteScenario("")
	if got := st.MetricsSnapshot(); got.UploadBatches != 0 {
		t.Fatalf("metrics after full reset = %#v, want zero", got)
	}
}

func TestListIncidentsFiltersScenario(t *testing.T) {
	st := &Store{}
	st.AddIncident(&incidentv1.Incident{Id: "i1", Scenario: "apt-fileless-c2"})
	st.AddIncident(&incidentv1.Incident{Id: "i2", Scenario: "benign-ci-noise"})

	got := st.ListIncidents("apt-fileless-c2")
	if len(got) != 1 || got[0].GetId() != "i1" {
		t.Fatalf("expected incident i1, got %#v", got)
	}
}

func TestUpsertsDuplicateEventsSignalsAndIncidents(t *testing.T) {
	st := &Store{}
	st.AddEvent(testEvent("ev-1", "a"))
	st.AddEvent(testEvent("ev-1", "a"))
	if got := st.ListEvents("a", ""); len(got) != 1 {
		t.Fatalf("events after duplicate upsert = %d, want 1", len(got))
	}

	sig := testSignal("sig-a", "a", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "payload_dropped", "lin-a", "file:/tmp/x")
	st.AddSignal(sig)
	st.AddSignal(testSignal("sig-b", "a", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "payload_dropped", "lin-a", "file:/tmp/x"))
	st.AddSignal(testSignal("sig-c", "a", signalv1.SignalWhere_SIGNAL_WHERE_CLOUD, "payload_dropped", "lin-a", "file:/tmp/x"))
	st.AddSignal(testSignal("sig-d", "a", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "payload_dropped", "lin-a", "file:/tmp/y"))
	if got := st.ListSignals("a", "", false); len(got) != 3 {
		t.Fatalf("signals after semantic duplicate upsert = %d, want 3", len(got))
	}

	inc := &incidentv1.Incident{
		Id:                  "inc-a",
		Scenario:            "a",
		Summary:             "same story",
		LineageIds:          []string{"lin-a", "lin-b"},
		Converge:            &incidentv1.ConvergeTrace{Method: "rarity+causal-topk"},
		ContributingSignals: []*signalv1.Signal{sig},
	}
	st.AddIncident(inc)
	st.AddIncident(&incidentv1.Incident{
		Id:                  "inc-b",
		Scenario:            "a",
		Summary:             "same story",
		LineageIds:          []string{"lin-b", "lin-a"},
		Converge:            &incidentv1.ConvergeTrace{Method: "rarity+causal-topk"},
		ContributingSignals: []*signalv1.Signal{sig},
	})
	if got := st.ListIncidents("a"); len(got) != 1 {
		t.Fatalf("incidents after semantic duplicate upsert = %d, want 1", len(got))
	}
}

func TestAgentHealthUpsertAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st.UpsertAgentHealth(agenthealth.AgentHealth{
		AgentID:    "agent-a",
		HostID:     "host-a",
		TenantID:   "default",
		Status:     "ok",
		ObservedAt: time.Now().UTC(),
		Sensor:     agenthealth.SensorHealth{Backend: "fake", Running: true, EventsSeen: 1},
	})
	st.UpsertAgentHealth(agenthealth.AgentHealth{
		AgentID:    "agent-a",
		HostID:     "host-a",
		TenantID:   "default",
		Status:     "degraded",
		ObservedAt: time.Now().UTC(),
		Sensor:     agenthealth.SensorHealth{Backend: "fake", Running: true, EventsSeen: 2},
	})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.GetAgentHealth("default", "agent-a")
	if !ok {
		t.Fatal("agent health not found")
	}
	if got.Status != "degraded" || got.Sensor.EventsSeen != 2 {
		t.Fatalf("health = %+v", got)
	}
	if got := reloaded.ListAgentHealth(); len(got) != 1 {
		t.Fatalf("health list len = %d, want 1", len(got))
	}
}

func TestDeleteScenario(t *testing.T) {
	st := &Store{}
	st.AddSignal(&signalv1.Signal{Id: "s1", Scenario: "a"})
	st.AddSignal(&signalv1.Signal{Id: "s2", Scenario: "b"})
	st.AddIncident(&incidentv1.Incident{Id: "i1", Scenario: "a"})
	st.AddIncident(&incidentv1.Incident{Id: "i2", Scenario: "b"})

	st.DeleteScenario("a")

	if got := st.ListSignals("a", "", false); len(got) != 0 {
		t.Fatalf("signals for deleted scenario = %d, want 0", len(got))
	}
	if got := st.ListIncidents("a"); len(got) != 0 {
		t.Fatalf("incidents for deleted scenario = %d, want 0", len(got))
	}
	if got := st.ListSignals("b", "", false); len(got) != 1 {
		t.Fatalf("signals for other scenario = %d, want 1", len(got))
	}
	if got := st.ListIncidents("b"); len(got) != 1 {
		t.Fatalf("incidents for other scenario = %d, want 1", len(got))
	}
}

func testEvent(id, scenario string) *eventv1.CanonicalEvent {
	return &eventv1.CanonicalEvent{Id: id, Scenario: scenario}
}

func testSignal(id, scenario string, where signalv1.SignalWhere, name, lineage, entity string) *signalv1.Signal {
	return &signalv1.Signal{
		Id:        id,
		Scenario:  scenario,
		Where:     where,
		Name:      name,
		LineageId: lineage,
		Entities:  []*signalv1.EntityRef{{Kind: "file", Key: entity, Role: "object"}},
		EventRefs: []string{"ev-1"},
	}
}
