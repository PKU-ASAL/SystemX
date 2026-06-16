package store

import (
	"path/filepath"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
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

func TestStoreInfoReportsBackendAndVersions(t *testing.T) {
	memory := (&Store{}).Info()
	if memory.Backend != "memory" || memory.Path != "" || memory.StateVersion == 0 || memory.PostgresSchema == 0 {
		t.Fatalf("memory store info = %+v", memory)
	}
	path := filepath.Join(t.TempDir(), "store.json")
	fileStore, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	file := fileStore.Info()
	if file.Backend != "file" || file.Path != path || file.MigrationVersion != FileStoreStateVersion {
		t.Fatalf("file store info = %+v", file)
	}
}

func TestIncidentLifecycleStatusPersistsAcrossUpsert(t *testing.T) {
	st := &Store{}
	inc := &incidentv1.Incident{
		Id:                  "inc-a",
		Scenario:            "a",
		Summary:             "same story",
		LineageIds:          []string{"lin-a"},
		Converge:            &incidentv1.ConvergeTrace{Method: "rarity+causal-topk"},
		ContributingSignals: []*signalv1.Signal{testSignal("sig-a", "a", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "reverse_shell_pattern", "lin-a", "process:p-bash")},
	}
	st.AddIncident(inc)
	if got := st.ListIncidents("a")[0].GetStatus(); got != "open" {
		t.Fatalf("default status = %q, want open", got)
	}
	if _, ok := st.UpdateIncidentStatus("", "a", "suppressed", "known test", "tester"); !ok {
		t.Fatal("UpdateIncidentStatus ok = false")
	}
	st.AddIncident(&incidentv1.Incident{
		Id:                  "inc-b",
		Scenario:            "a",
		Summary:             "same story",
		LineageIds:          []string{"lin-a"},
		Converge:            &incidentv1.ConvergeTrace{Method: "rarity+causal-topk"},
		ContributingSignals: inc.GetContributingSignals(),
	})
	got := st.ListIncidents("a")[0]
	if got.GetStatus() != "suppressed" || got.GetStatusReason() != "known test" || got.GetStatusActor() != "tester" {
		t.Fatalf("status after upsert = %q/%q/%q", got.GetStatus(), got.GetStatusReason(), got.GetStatusActor())
	}
}

func TestIncidentEvidenceAttachPersistsAcrossUpsert(t *testing.T) {
	st := &Store{}
	sig := testSignal("sig-a", "a", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "reverse_shell_pattern", "lin-a", "process:p-bash")
	inc := &incidentv1.Incident{
		Id:                  "inc-a",
		Scenario:            "a",
		Summary:             "same story",
		LineageIds:          []string{"lin-a"},
		Evidence:            &incidentv1.EvidenceSubgraph{Nodes: []*incidentv1.GraphNode{{Id: "process:p-bash", Kind: "process"}}},
		Converge:            &incidentv1.ConvergeTrace{Method: "rarity+causal-topk"},
		ContributingSignals: []*signalv1.Signal{sig},
	}
	st.AddIncident(inc)
	if _, ok := st.AttachIncidentEvidence("", "a", &incidentv1.EvidenceSubgraph{
		Nodes: []*incidentv1.GraphNode{
			{Id: "user:root", Kind: "user", Label: "root"},
			{Id: "process:p-bash", Kind: "process"},
		},
		Edges: []*incidentv1.GraphEdge{{From: "process:p-bash", To: "user:root", Kind: "ran_as"}},
	}); !ok {
		t.Fatal("AttachIncidentEvidence ok = false")
	}
	st.AddIncident(&incidentv1.Incident{
		Id:                  "inc-b",
		Scenario:            "a",
		Summary:             "same story",
		LineageIds:          []string{"lin-a"},
		Evidence:            &incidentv1.EvidenceSubgraph{Nodes: []*incidentv1.GraphNode{{Id: "process:p-bash", Kind: "process"}}},
		Converge:            &incidentv1.ConvergeTrace{Method: "rarity+causal-topk"},
		ContributingSignals: []*signalv1.Signal{sig},
	})
	got := st.ListIncidents("a")[0].GetEvidence()
	if len(got.GetNodes()) != 2 {
		t.Fatalf("nodes after attach/upsert = %d, want 2: %+v", len(got.GetNodes()), got.GetNodes())
	}
	if len(got.GetEdges()) != 1 || got.GetEdges()[0].GetKind() != "ran_as" {
		t.Fatalf("edges after attach/upsert = %+v", got.GetEdges())
	}
}

func TestMergeIncidentsCombinesEvidenceAndRemovesSource(t *testing.T) {
	st := &Store{}
	st.AddIncident(&incidentv1.Incident{
		Id:         "inc-a",
		Scenario:   "a",
		Summary:    "target",
		Severity:   40,
		Mitre:      []string{"T1059"},
		LineageIds: []string{"lin-a"},
		Terminals:  []string{"process:p-a"},
		Evidence:   &incidentv1.EvidenceSubgraph{Nodes: []*incidentv1.GraphNode{{Id: "process:p-a", Kind: "process"}}},
		ContributingSignals: []*signalv1.Signal{
			testSignal("sig-a", "a", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "payload_dropped", "lin-a", "file:/tmp/a"),
		},
		Status: "suppressed",
	})
	st.AddIncident(&incidentv1.Incident{
		Id:         "inc-b",
		Scenario:   "b",
		Summary:    "source",
		Severity:   80,
		Mitre:      []string{"T1105"},
		LineageIds: []string{"lin-b"},
		Terminals:  []string{"process:p-b"},
		Evidence: &incidentv1.EvidenceSubgraph{
			Nodes: []*incidentv1.GraphNode{{Id: "process:p-b", Kind: "process"}},
			Edges: []*incidentv1.GraphEdge{{From: "process:p-a", To: "process:p-b", Kind: "related"}},
		},
		ContributingSignals: []*signalv1.Signal{
			testSignal("sig-b", "b", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "reverse_shell_pattern", "lin-b", "process:p-b"),
		},
	})
	merged, ok := st.MergeIncidents("inc-a", "inc-b")
	if !ok {
		t.Fatal("MergeIncidents ok = false")
	}
	if merged.GetStatus() != "suppressed" {
		t.Fatalf("target status = %q, want suppressed", merged.GetStatus())
	}
	if merged.GetSeverity() != 80 {
		t.Fatalf("severity = %d, want 80", merged.GetSeverity())
	}
	if len(merged.GetLineageIds()) != 2 || len(merged.GetTerminals()) != 2 || len(merged.GetMitre()) != 2 {
		t.Fatalf("merged fields incomplete: lineage=%v terminals=%v mitre=%v", merged.GetLineageIds(), merged.GetTerminals(), merged.GetMitre())
	}
	if len(merged.GetEvidence().GetNodes()) != 2 || len(merged.GetEvidence().GetEdges()) != 1 {
		t.Fatalf("merged evidence = %+v", merged.GetEvidence())
	}
	if len(merged.GetContributingSignals()) != 2 {
		t.Fatalf("contributing signals = %d, want 2", len(merged.GetContributingSignals()))
	}
	if got := st.ListIncidents(""); len(got) != 1 || got[0].GetId() != "inc-a" {
		t.Fatalf("incidents after merge = %+v", got)
	}
}

func TestUpsertsDuplicateEventsSignalsAndIncidents(t *testing.T) {
	st := &Store{}
	if inserted := st.AddEvent(testEvent("ev-1", "a")); !inserted {
		t.Fatal("first AddEvent inserted = false")
	}
	if inserted := st.AddEvent(testEvent("ev-1", "a")); inserted {
		t.Fatal("duplicate AddEvent inserted = true")
	}
	if got := st.ListEvents("a", ""); len(got) != 1 {
		t.Fatalf("events after duplicate upsert = %d, want 1", len(got))
	}

	sig := testSignal("sig-a", "a", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "payload_dropped", "lin-a", "file:/tmp/x")
	if inserted := st.AddSignal(sig); !inserted {
		t.Fatal("first AddSignal inserted = false")
	}
	if inserted := st.AddSignal(testSignal("sig-b", "a", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "payload_dropped", "lin-a", "file:/tmp/x")); inserted {
		t.Fatal("duplicate AddSignal inserted = true")
	}
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
	if inserted := st.AddIncident(inc); !inserted {
		t.Fatal("first AddIncident inserted = false")
	}
	if inserted := st.AddIncident(&incidentv1.Incident{
		Id:                  "inc-b",
		Scenario:            "a",
		Summary:             "same story",
		LineageIds:          []string{"lin-b", "lin-a"},
		Converge:            &incidentv1.ConvergeTrace{Method: "rarity+causal-topk"},
		ContributingSignals: []*signalv1.Signal{sig},
	}); inserted {
		t.Fatal("duplicate AddIncident inserted = true")
	}
	if got := st.ListIncidents("a"); len(got) != 1 {
		t.Fatalf("incidents after semantic duplicate upsert = %d, want 1", len(got))
	}
}

func TestAddAgentSeparatesTenants(t *testing.T) {
	st := &Store{}
	st.AddAgent(&analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", TenantId: "tenant-a"})
	st.AddAgent(&analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-b", TenantId: "tenant-b"})
	st.AddAgent(&analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a2", TenantId: "tenant-a"})
	agents := st.ListAgents()
	if len(agents) != 2 {
		t.Fatalf("agents = %d, want 2 tenant-scoped entries", len(agents))
	}
	for _, agent := range agents {
		if agent.GetTenantId() == "tenant-a" && agent.GetHostId() != "host-a2" {
			t.Fatalf("tenant-a agent was not updated: %+v", agent)
		}
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
		Scope:      agenthealth.RuntimeScope{Type: "container", Selector: "abc123"},
		Status:     "ok",
		ObservedAt: time.Now().UTC(),
		Capability: agenthealth.SensorCapability{Backend: "fake", KernelRelease: "test-kernel", BTFAvailable: true, BPFFSAvailable: true},
		Sensor:     agenthealth.SensorHealth{Backend: "fake", Running: true, EventsSeen: 1},
	})
	st.UpsertAgentHealth(agenthealth.AgentHealth{
		AgentID:    "agent-a",
		HostID:     "host-a",
		TenantID:   "default",
		Scope:      agenthealth.RuntimeScope{Type: "container", Selector: "abc123"},
		Status:     "degraded",
		ObservedAt: time.Now().UTC(),
		Capability: agenthealth.SensorCapability{Backend: "fake", KernelRelease: "test-kernel", BTFAvailable: true, BPFFSAvailable: true},
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
	if got.Status != "degraded" || got.Sensor.EventsSeen != 2 || got.Scope.Type != "container" || got.Scope.Selector != "abc123" || got.Capability.KernelRelease != "test-kernel" || !got.Capability.BTFAvailable || !got.Capability.BPFFSAvailable {
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

func TestPolicyAssignmentAndEffectivePolicy(t *testing.T) {
	st := &Store{}
	st.EnsureDefaultPolicy("default")
	if rules := st.ListRules(""); len(rules) == 0 {
		t.Fatal("default rules were not seeded")
	}
	if policy, ok := st.EffectivePolicy("default", "agent-a", "container", "abc123"); !ok || policy.PolicyID != policymodel.DefaultPolicyID || policy.Version != policymodel.DefaultPolicyVersion {
		t.Fatalf("default effective policy = %+v ok=%t", policy, ok)
	}

	custom := policymodel.DefaultPolicy("default")
	custom.PolicyID = "cloud-no-cross"
	custom.Version = 2
	custom.CloudRules = []string{"web_shell_chain"}
	st.UpsertPolicy(custom)
	assignment, ok := st.AssignPolicy(policymodel.Assignment{
		TenantID:      "default",
		Scope:         policymodel.ScopeSelector{Type: "container", Selector: "abc123"},
		PolicyID:      "cloud-no-cross",
		PolicyVersion: 2,
	})
	if !ok || assignment.PolicyVersion != 2 {
		t.Fatalf("assignment = %+v ok=%t", assignment, ok)
	}
	policy, ok := st.EffectivePolicy("default", "agent-a", "container", "abc123")
	if !ok || policy.PolicyID != "cloud-no-cross" || len(policy.CloudRules) != 1 || policy.CloudRules[0] != "web_shell_chain" {
		t.Fatalf("effective scoped policy = %+v ok=%t", policy, ok)
	}
	other, ok := st.EffectivePolicy("default", "agent-a", "container", "other")
	if !ok || other.PolicyID != policymodel.DefaultPolicyID {
		t.Fatalf("effective fallback policy = %+v ok=%t", other, ok)
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
