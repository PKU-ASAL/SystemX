package store

import (
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/agentplane/model"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"path/filepath"
	"testing"
	"time"
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
	st.RecordDataBatchIngest(2, 3, 1, 1, 12*time.Millisecond)
	st.RecordDataBatchIngest(1, 1, 0, 0, 4*time.Millisecond)

	got := st.MetricsSnapshot()
	if got.DataBatchesAppended != 2 {
		t.Fatalf("data batches appended = %d, want 2", got.DataBatchesAppended)
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
	if got := st.MetricsSnapshot(); got.DataBatchesAppended != 0 {
		t.Fatalf("metrics after full reset = %#v, want zero", got)
	}
}

func TestEvidencePullbacksPersistAcrossStateExport(t *testing.T) {
	st := &Store{}
	st.CreateEvidencePullback(controlmodel.EvidencePullbackRequest{
		RequestID:  "evpb-a",
		TenantID:   "default",
		AgentID:    "agent-a",
		IncidentID: "inc-a",
		Target:     "process:p1",
	})
	state, err := st.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	restored := &Store{}
	if err := restored.ImportState(state); err != nil {
		t.Fatal(err)
	}
	got := restored.PendingEvidencePullbacks("default", "agent-a")
	if len(got) != 1 {
		t.Fatalf("pullbacks = %+v", got)
	}
	if got[0].RequestID != "evpb-a" || got[0].Status != controlmodel.EvidencePullbackStatusPending {
		t.Fatalf("pullback = %+v", got[0])
	}
}

func TestApproveResponseMovesPendingApprovalToPending(t *testing.T) {
	st := &Store{}
	st.CreateResponse(responsemodel.Command{
		ResponseID:       "resp-approve",
		TenantID:         "default",
		AgentID:          "agent-a",
		Action:           "collect",
		Mode:             "observe",
		Status:           "pending_approval",
		ApprovalRequired: true,
		ApprovalStatus:   "required",
	})
	if got := st.PendingResponses("default", "agent-a"); len(got) != 0 {
		t.Fatalf("pending before approval = %+v", got)
	}
	cmd, ok := st.ApproveResponse("default", "agent-a", "resp-approve", true, "analyst", "", "approved for collection")
	if !ok {
		t.Fatal("ApproveResponse ok = false")
	}
	if cmd.Status != "pending" || cmd.ApprovalStatus != "approved" || cmd.ApprovedBy != "analyst" || cmd.ApprovedAt.IsZero() {
		t.Fatalf("approved command = %+v", cmd)
	}
	if got := st.PendingResponses("default", "agent-a"); len(got) != 1 || got[0].ResponseID != "resp-approve" {
		t.Fatalf("pending after approval = %+v", got)
	}
}

func TestApproveResponseRejectsNonApprovalCommand(t *testing.T) {
	st := &Store{}
	st.CreateResponse(responsemodel.Command{
		ResponseID: "resp-denied",
		TenantID:   "default",
		AgentID:    "agent-a",
		Action:     "kill",
		Mode:       "observe",
		Status:     "denied",
	})
	if _, ok := st.ApproveResponse("default", "agent-a", "resp-denied", true, "analyst", "", "no bypass"); ok {
		t.Fatal("ApproveResponse ok = true for non-approval command")
	}
	audits := st.ListResponses("default", "agent-a")
	if len(audits) != 1 || audits[0].Command.Status != "denied" {
		t.Fatalf("audits = %+v", audits)
	}
}

func TestApproveResponseRequiresThresholdAndAllowedRole(t *testing.T) {
	st := &Store{}
	st.CreateResponse(responsemodel.Command{
		ResponseID:        "resp-multi-approve",
		TenantID:          "default",
		AgentID:           "agent-a",
		Action:            "collect",
		Mode:              "observe",
		Status:            "pending_approval",
		ApprovalRequired:  true,
		ApprovalStatus:    "required",
		ApprovalThreshold: 2,
		ApprovalRoles:     []string{"responder", "security_admin"},
	})
	if _, ok := st.ApproveResponse("default", "agent-a", "resp-multi-approve", true, "observer", "viewer", "wrong role"); ok {
		t.Fatal("ApproveResponse ok = true for wrong role")
	}
	cmd, ok := st.ApproveResponse("default", "agent-a", "resp-multi-approve", true, "responder-a", "responder", "first approval")
	if !ok {
		t.Fatal("first ApproveResponse ok = false")
	}
	if cmd.Status != "pending_approval" || cmd.ApprovalStatus != "partial" || len(cmd.Approvals) != 1 {
		t.Fatalf("after first approval = %+v", cmd)
	}
	if got := st.PendingResponses("default", "agent-a"); len(got) != 0 {
		t.Fatalf("pending after partial approval = %+v", got)
	}
	cmd, ok = st.ApproveResponse("default", "agent-a", "resp-multi-approve", true, "security-b", "security_admin", "second approval")
	if !ok {
		t.Fatal("second ApproveResponse ok = false")
	}
	if cmd.Status != "pending" || cmd.ApprovalStatus != "approved" || len(cmd.Approvals) != 2 || cmd.ApprovedBy != "security-b" {
		t.Fatalf("after second approval = %+v", cmd)
	}
	if got := st.PendingResponses("default", "agent-a"); len(got) != 1 || got[0].ResponseID != "resp-multi-approve" {
		t.Fatalf("pending after threshold approval = %+v", got)
	}
}

func TestCompleteEvidencePullbackUpdatesStatus(t *testing.T) {
	st := &Store{}
	st.CreateEvidencePullback(controlmodel.EvidencePullbackRequest{
		RequestID: "evpb-a",
		TenantID:  "default",
		AgentID:   "agent-a",
	})
	req, ok := st.CompleteEvidencePullback(controlmodel.EvidencePullbackResult{
		RequestID: "evpb-a",
		TenantID:  "default",
		AgentID:   "agent-a",
		OK:        true,
		Message:   "collected",
	})
	if !ok {
		t.Fatal("CompleteEvidencePullback ok = false")
	}
	if req.Status != controlmodel.EvidencePullbackStatusCompleted || !req.ResultOK || req.Result != "collected" || req.CompletedAt.IsZero() {
		t.Fatalf("completed request = %+v", req)
	}
	if got := st.PendingEvidencePullbacks("default", "agent-a"); len(got) != 0 {
		t.Fatalf("pending after completion = %+v", got)
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
	st.AddAgent(AgentIdentity{AgentID: "agent-a", HostID: "host-a", TenantID: "tenant-a"})
	st.AddAgent(AgentIdentity{AgentID: "agent-a", HostID: "host-b", TenantID: "tenant-b"})
	st.AddAgent(AgentIdentity{AgentID: "agent-a", HostID: "host-a2", TenantID: "tenant-a"})
	agents := st.ListAgents()
	if len(agents) != 2 {
		t.Fatalf("agents = %d, want 2 tenant-scoped entries", len(agents))
	}
	for _, agent := range agents {
		if agent.TenantID == "tenant-a" && agent.HostID != "host-a2" {
			t.Fatalf("tenant-a agent was not updated: %+v", agent)
		}
	}
}

func TestBindAgentIdentityLocksCertificatePrincipal(t *testing.T) {
	st := &Store{}
	if err := st.BindAgentIdentity(AgentIdentity{
		AgentID:      "agent-a",
		TenantID:     "tenant-a",
		AuthType:     "mtls",
		CertIdentity: "spiffe://sysarmor.local/tenant/tenant-a/agent/agent-a",
	}); err != nil {
		t.Fatalf("BindAgentIdentity() error = %v", err)
	}
	st.AddAgent(AgentIdentity{AgentID: "agent-a", TenantID: "tenant-a", HostID: "host-a", Version: "v1"})
	agents := st.ListAgents()
	if len(agents) != 1 || agents[0].CertIdentity == "" || agents[0].AuthType != "mtls" || agents[0].HostID != "host-a" {
		t.Fatalf("agents = %+v, want preserved mTLS binding with updated host", agents)
	}
	err := st.BindAgentIdentity(AgentIdentity{
		AgentID:      "agent-a",
		TenantID:     "tenant-a",
		AuthType:     "mtls",
		CertIdentity: "spiffe://sysarmor.local/tenant/tenant-a/agent/agent-b",
	})
	if err == nil {
		t.Fatal("BindAgentIdentity() mismatched cert identity succeeded")
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

func TestExportImportStateRoundTrip(t *testing.T) {
	st := &Store{}
	st.AddAgent(AgentIdentity{AgentID: "agent-a", HostID: "host-a", TenantID: "default", Version: "test"})
	st.AddEvent(testEvent("ev-a", "scenario-a"))
	st.AddSignal(testSignal("sig-a", "scenario-a", signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT, "reverse_shell_pattern", "lin-a", "process:p-bash"))
	st.AddIncident(&incidentv1.Incident{Id: "inc-a", Scenario: "scenario-a", Summary: "incident-a", Status: "open"})
	st.UpsertAgentHealth(agenthealth.AgentHealth{AgentID: "agent-a", HostID: "host-a", TenantID: "default", Status: "ok"})
	st.RecordDataBatchAppend(AgentIdentity{AgentID: "agent-a", TenantID: "default"}, "batch-a", "http", time.Unix(10, 0).UTC())
	st.UpsertOperatorRoleBinding(OperatorRoleBinding{Actor: "alice", Roles: []string{"policy_admin", "policy_admin", "responder"}})
	st.RecordDataBatchIngest(1, 1, 1, 1, time.Millisecond)
	st.ObserveRaritySignals([]*signalv1.Signal{{
		Name: "download_by_lolbin",
		Entities: []*signalv1.EntityRef{{
			Kind: "container",
			Key:  "checkout-api",
		}},
	}})

	state, err := st.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	reloaded := &Store{}
	if err := reloaded.ImportState(state); err != nil {
		t.Fatal(err)
	}
	if got := reloaded.ListAgents(); len(got) != 1 || got[0].AgentID != "agent-a" {
		t.Fatalf("agents after import = %+v", got)
	}
	if got := reloaded.ListEvents("scenario-a", ""); len(got) != 1 || got[0].GetId() != "ev-a" {
		t.Fatalf("events after import = %+v", got)
	}
	if got := reloaded.ListSignals("scenario-a", "endpoint", false); len(got) != 1 || got[0].GetId() != "sig-a" {
		t.Fatalf("signals after import = %+v", got)
	}
	if got := reloaded.ListIncidents("scenario-a"); len(got) != 1 || got[0].GetId() != "inc-a" {
		t.Fatalf("incidents after import = %+v", got)
	}
	if _, ok := reloaded.GetAgentHealth("default", "agent-a"); !ok {
		t.Fatal("agent health missing after import")
	}
	if got := reloaded.ListAgentSessions("default", "agent-a"); len(got) != 1 || got[0].LastAckCursor != "batch-a" {
		t.Fatalf("agent sessions after import = %+v", got)
	}
	if got, ok := reloaded.OperatorRolesForActor("alice"); !ok || len(got) != 2 || got[0] != "policy_admin" || got[1] != "responder" {
		t.Fatalf("operator roles after import = %+v ok=%v", got, ok)
	}
	if got := reloaded.MetricsSnapshot(); got.DataBatchesAppended != 1 || got.SignalsEmitted != 2 {
		t.Fatalf("metrics after import = %+v", got)
	}
	if got := reloaded.RarityBaselineSnapshot().Count("container:checkout-api", "download_by_lolbin"); got != 1 {
		t.Fatalf("rarity baseline after import = %d, want 1", got)
	}
}

func TestRecordDataBatchAppendUpdatesSessionCursor(t *testing.T) {
	st := &Store{}
	agent := AgentIdentity{AgentID: "agent-a", TenantID: "default"}
	first := st.RecordDataBatchAppend(agent, "batch-1", "http", time.Unix(10, 0).UTC())
	second := st.RecordDataBatchAppend(agent, "batch-2", "grpc", time.Unix(20, 0).UTC())
	if first.SessionID == "" || first.SessionID != second.SessionID {
		t.Fatalf("session ids = %q/%q", first.SessionID, second.SessionID)
	}
	sessions := st.ListAgentSessions("default", "agent-a")
	if len(sessions) != 1 {
		t.Fatalf("sessions len = %d, want 1", len(sessions))
	}
	got := sessions[0]
	if got.StartedAt != first.StartedAt || got.LastSeenAt != second.LastSeenAt || got.LastAckCursor != "batch-2" || got.DataTransport != "grpc" {
		t.Fatalf("session after update = %+v", got)
	}
}

func TestAgentSessionLifecycle(t *testing.T) {
	st := &Store{}
	opened := st.RecordControlSessionOpen("default", "agent-control", "control", time.Unix(10, 0).UTC())
	if opened.Status != "open" || opened.ControlTransport != "control" || !opened.ClosedAt.IsZero() {
		t.Fatalf("opened session = %+v", opened)
	}
	seen := st.RecordAgentSessionSeen("default", "agent-control", time.Unix(20, 0).UTC())
	if seen.Status != "open" || !seen.LastSeenAt.Equal(time.Unix(20, 0).UTC()) {
		t.Fatalf("seen session = %+v", seen)
	}
	st.RecordDataBatchAppend(AgentIdentity{AgentID: "agent-control", TenantID: "default"}, "batch-grpc", "grpc", time.Unix(25, 0).UTC())
	closed := st.CloseAgentSession("default", "agent-control", time.Unix(30, 0).UTC())
	if closed.Status != "closed" || !closed.ClosedAt.Equal(time.Unix(30, 0).UTC()) || closed.LastAckCursor != "batch-grpc" {
		t.Fatalf("closed session = %+v", closed)
	}
	reopened := st.RecordControlSessionOpen("default", "agent-control", "control", time.Unix(40, 0).UTC())
	if reopened.Status != "open" || reopened.ControlTransport != "control" || !reopened.ClosedAt.IsZero() || reopened.LastAckCursor != "batch-grpc" {
		t.Fatalf("reopened session = %+v", reopened)
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

func TestPolicyDraftMustBePublishedBeforeAssignment(t *testing.T) {
	st := &Store{}
	st.EnsureDefaultPolicy("default")
	draft := policymodel.DefaultPolicy("default")
	draft.PolicyID = "draft-policy"
	draft.Version = 2
	draft.Published = false
	st.UpsertPolicy(draft)
	if _, ok := st.AssignPolicy(policymodel.Assignment{
		TenantID:      "default",
		AgentID:       "agent-a",
		PolicyID:      "draft-policy",
		PolicyVersion: 2,
	}); ok {
		t.Fatal("AssignPolicy ok = true for draft policy")
	}
	if effective, ok := st.EffectivePolicy("default", "agent-a", "", ""); !ok || effective.PolicyID != policymodel.DefaultPolicyID {
		t.Fatalf("effective policy = %+v ok=%t", effective, ok)
	}
	published, ok := st.PublishPolicy("default", "draft-policy", 2, true)
	if !ok || !published.Published {
		t.Fatalf("PublishPolicy = %+v ok=%t", published, ok)
	}
	if _, ok := st.AssignPolicy(policymodel.Assignment{
		TenantID:      "default",
		AgentID:       "agent-a",
		PolicyID:      "draft-policy",
		PolicyVersion: 2,
	}); !ok {
		t.Fatal("AssignPolicy ok = false after publish")
	}
	if effective, ok := st.EffectivePolicy("default", "agent-a", "", ""); !ok || effective.PolicyID != "draft-policy" || !effective.Published {
		t.Fatalf("effective policy = %+v ok=%t", effective, ok)
	}
}

func TestPolicyAuditPersistsAcrossStateExport(t *testing.T) {
	st := &Store{}
	st.RecordPolicyAudit(policymodel.AuditRecord{
		TenantID:      "default",
		Action:        "policy.publish",
		PolicyID:      "policy-a",
		PolicyVersion: 2,
		Actor:         "analyst",
	})
	state, err := st.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	restored := &Store{}
	if err := restored.ImportState(state); err != nil {
		t.Fatal(err)
	}
	got := restored.ListPolicyAudits("default", "policy-a")
	if len(got) != 1 || got[0].Action != "policy.publish" || got[0].Actor != "analyst" || got[0].AuditID == "" {
		t.Fatalf("policy audit = %+v", got)
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
