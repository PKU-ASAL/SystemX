package managerapi

import (
	"context"
	"encoding/json"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
	platformopensearch "github.com/sysarmor/sysarmor-next-project/internal/platform/opensearch"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	ingestworker "github.com/sysarmor/sysarmor-next-project/internal/workers/ingest"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type recordingIndexer struct {
	docs []platformopensearch.Document
}

func (i *recordingIndexer) Index(_ context.Context, doc platformopensearch.Document) error {
	i.docs = append(i.docs, doc)
	return nil
}

type fakeSearcher struct {
	docs map[string][]json.RawMessage
}

func (s fakeSearcher) Search(_ context.Context, index string, _ int) ([]json.RawMessage, error) {
	return append([]json.RawMessage(nil), s.docs[index]...), nil
}

func newTestServer(st *store.Store) *Server {
	return NewServer(st)
}

func TestSearchBackedTelemetryQueries(t *testing.T) {
	st := &store.Store{}
	searcher := fakeSearcher{docs: map[string][]json.RawMessage{
		"sysarmor-events": {
			json.RawMessage(`{"id":"ev-a","behavior":"process.exec","labels":{"scenario":"managed"}}`),
			json.RawMessage(`{"id":"ev-b","behavior":"file.write","labels":{"scenario":"other"}}`),
		},
		"sysarmor-signals": {
			json.RawMessage(`{"id":"sig-a","name":"payload_dropped","where":"SIGNAL_WHERE_ENDPOINT","terminal":false,"labels":{"scenario":"managed"}}`),
			json.RawMessage(`{"id":"sig-b","name":"web_shell_chain","where":"SIGNAL_WHERE_CLOUD","terminal":true,"labels":{"scenario":"managed"}}`),
		},
		"sysarmor-incidents": {
			json.RawMessage(`{"id":"inc-a","summary":"incident","labels":{"scenario":"managed"}}`),
		},
	}}
	handler := NewServerWithSearch(st, "", searcher).Handler()

	rec := get(t, handler, "/api/v1/events?label=scenario=managed&behavior=process.exec")
	if !strings.Contains(rec.Body.String(), `"id":"ev-a"`) || strings.Contains(rec.Body.String(), `"id":"ev-b"`) {
		t.Fatalf("search-backed events mismatch: %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/signals?label=scenario=managed&layer=cloud&terminal=true")
	if !strings.Contains(rec.Body.String(), `"id":"sig-b"`) || strings.Contains(rec.Body.String(), `"id":"sig-a"`) {
		t.Fatalf("search-backed signals mismatch: %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/incidents?label=scenario=managed")
	if !strings.Contains(rec.Body.String(), `"id":"inc-a"`) {
		t.Fatalf("search-backed incidents mismatch: %s", rec.Body.String())
	}
}

func TestUploadTriggersAnalyticsAndQueries(t *testing.T) {
	st := &store.Store{}
	srv := newTestServer(st)
	handler := srv.Handler()
	batch := httpDataBatch("", "agent-a", "host-a", nil, []*signalv1.Signal{
		endpointSignal("web_runtime_spawns_shell", "lin-a", false, processEntity("p-web")),
		endpointSignal("payload_dropped", "lin-a", false, fileEntity("/dev/shm/x.sh")),
		endpointSignal("reverse_shell_pattern", "lin-a", true, processEntity("p-bash"), socketEntity("10.66.0.99:443")),
	})
	appendBatch(t, srv, batch)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/signals?label=scenario=apt-fileless-c2&layer=cloud", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("signals status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "web_shell_chain") {
		t.Fatalf("cloud signals missing web_shell_chain: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/incidents?label=scenario=apt-fileless-c2", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("incidents status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "rarity+causal-topk") {
		t.Fatalf("incident missing converge method: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/incident-evidence?label=scenario=apt-fileless-c2", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("incident evidence status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"nodes"`, `"edges"`, `"kind":"connect"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("incident evidence missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/incident-evidence?label=scenario=apt-fileless-c2&path_from=process:p-bash&path_to=socket:10.66.0.99:443")
	for _, want := range []string{`"id":"process:p-bash"`, `"id":"socket:10.66.0.99:443"`, `"kind":"connect"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("incident path missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/incident-evidence?label=scenario=apt-fileless-c2&seed=process:p-bash&hops=1")
	if !strings.Contains(rec.Body.String(), `"id":"socket:10.66.0.99:443"`) {
		t.Fatalf("incident k-hop missing socket node: %s", rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/metrics")
	for _, want := range []string{
		`"data_batches_appended":1`,
		`"endpoint_signals_ingested":3`,
		`"cloud_signals_emitted":2`,
		`"signals_emitted":5`,
		`"incidents_created":1`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("metrics missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestStoreStatusAPI(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	rec := get(t, handler, "/healthz")
	for _, want := range []string{`"ok":true`, `"store"`, `"backend":"memory"`, `"postgres_schema_version":1`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("healthz missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/store-status")
	for _, want := range []string{`"backend":"memory"`, `"state_version":1`, `"migration_version":1`, `"postgres_schema_version":1`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("store status missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestIncidentLifecycleAPIUpdatesStatus(t *testing.T) {
	st := &store.Store{}
	st.AddIncident(&incidentv1.Incident{Id: "inc-a", Labels: labelsForScenario("scenario-a"), Summary: "test incident"})
	handler := NewServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/incident-lifecycle", strings.NewReader(`{"labels":{"scenario":"scenario-a"},"status":"closed","reason":"triaged","actor":"analyst"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("lifecycle status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"status":"closed"`, `"status_reason":"triaged"`, `"status_actor":"analyst"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("lifecycle response missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/incidents?label=scenario=scenario-a")
	if !strings.Contains(rec.Body.String(), `"status":"closed"`) {
		t.Fatalf("incident status not queryable: %s", rec.Body.String())
	}
}

func TestIncidentMergeAPI(t *testing.T) {
	st := &store.Store{}
	st.AddIncident(&incidentv1.Incident{
		Id:         "inc-a",
		Labels:     labelsForScenario("scenario-a"),
		Summary:    "target",
		LineageIds: []string{"lin-a"},
		Evidence:   &incidentv1.EvidenceSubgraph{Nodes: []*incidentv1.GraphNode{{Id: "process:p-a", Kind: "process"}}},
		Status:     "closed",
	})
	st.AddIncident(&incidentv1.Incident{
		Id:         "inc-b",
		Labels:     labelsForScenario("scenario-b"),
		Summary:    "source",
		LineageIds: []string{"lin-b"},
		Evidence:   &incidentv1.EvidenceSubgraph{Nodes: []*incidentv1.GraphNode{{Id: "process:p-b", Kind: "process"}}},
	})
	handler := NewServer(st).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/incident-merge", strings.NewReader(`{"target_incident_id":"inc-a","source_incident_id":"inc-b"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("merge status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"id":"inc-a"`, `"status":"closed"`, `"lin-b"`, `"id":"process:p-b"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("merge response missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/incidents")
	if strings.Contains(rec.Body.String(), `"id":"inc-b"`) {
		t.Fatalf("source incident still queryable: %s", rec.Body.String())
	}
}

func TestDataBatchAppendRecordsSessionCursor(t *testing.T) {
	st := &store.Store{}
	srv := newTestServer(st)
	handler := srv.Handler()
	batch := httpDataBatch("00000000000000000042", "agent-a", "host-a", []*eventv1.CanonicalEvent{{
		Id:       "ev-ack",
		Behavior: "process.exec",
	}}, nil)
	ack := appendBatchAndAck(t, srv, batch)
	if !ack.GetAccepted() || ack.GetStatus() != dataplanev1.DataAck_STATUS_ACCEPTED || ack.GetBatchId() != batch.GetHeader().GetBatchId() || ack.GetCommittedCursor() != batch.GetHeader().GetBatchId() || ack.GetAcceptedEvents() != 1 || ack.GetServerTime() == "" {
		t.Fatalf("ack = %#v", ack)
	}
	rec := get(t, handler, "/api/v1/agent-sessions?tenant_id=default&agent_id=agent-a")
	for _, want := range []string{`"tenant_id":"default"`, `"agent_id":"agent-a"`, `"last_ack_cursor":"00000000000000000042"`, `"data_transport":"grpc"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("agent session missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/data-resume?tenant_id=default&agent_id=agent-a")
	if !strings.Contains(rec.Body.String(), `"resume_cursor":"00000000000000000042"`) {
		t.Fatalf("resume cursor mismatch: %s", rec.Body.String())
	}
}

func TestQueryPagination(t *testing.T) {
	st := &store.Store{}
	st.AddEvent(&eventv1.CanonicalEvent{Id: "ev-1", Labels: labelsForScenario("page"), Behavior: "process.exec"})
	st.AddEvent(&eventv1.CanonicalEvent{Id: "ev-2", Labels: labelsForScenario("page"), Behavior: "file.open"})
	st.AddEvent(&eventv1.CanonicalEvent{Id: "ev-3", Labels: labelsForScenario("page"), Behavior: "network.connect"})
	st.AddSignal(endpointSignalForScenario("page", "sig-1", "lin-1", false, processEntity("p1")))
	st.AddSignal(endpointSignalForScenario("page", "sig-2", "lin-2", false, processEntity("p2")))
	st.AddIncident(&incidentv1.Incident{Id: "inc-1", Labels: labelsForScenario("page"), Summary: "one"})
	st.AddIncident(&incidentv1.Incident{Id: "inc-2", Labels: labelsForScenario("page"), Summary: "two"})
	handler := NewServer(st).Handler()

	rec := get(t, handler, "/api/v1/events?label=scenario=page&limit=1&offset=1")
	if strings.Contains(rec.Body.String(), `"id":"ev-1"`) || !strings.Contains(rec.Body.String(), `"id":"ev-2"`) || strings.Contains(rec.Body.String(), `"id":"ev-3"`) {
		t.Fatalf("events page mismatch: %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/signals?label=scenario=page&limit=1&offset=1")
	if strings.Contains(rec.Body.String(), `"name":"sig-1"`) || !strings.Contains(rec.Body.String(), `"name":"sig-2"`) {
		t.Fatalf("signals page mismatch: %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/incidents?label=scenario=page&limit=1&offset=1")
	if strings.Contains(rec.Body.String(), `"id":"inc-1"`) || !strings.Contains(rec.Body.String(), `"id":"inc-2"`) {
		t.Fatalf("incidents page mismatch: %s", rec.Body.String())
	}
}

func TestDataBatchAppendRetryIsIdempotentForAcceptedCounts(t *testing.T) {
	st := &store.Store{}
	srv := newTestServer(st)
	handler := srv.Handler()
	batch := httpDataBatch("00000000000000000007", "agent-a", "host-a", []*eventv1.CanonicalEvent{{
		Id:       "ev-retry",
		Labels:   labelsForScenario("apt-fileless-c2"),
		Behavior: "process.exec",
	}}, []*signalv1.Signal{
		endpointSignal("web_runtime_spawns_shell", "lin-retry", false, processEntity("p-web")),
		endpointSignal("payload_dropped", "lin-retry", false, fileEntity("/dev/shm/x.sh")),
		endpointSignal("reverse_shell_pattern", "lin-retry", true, processEntity("p-bash"), socketEntity("10.66.0.99:443")),
	})
	first := appendBatchAndAck(t, srv, batch)
	if !first.GetAccepted() {
		t.Fatalf("first ack = %#v", first)
	}
	second := appendBatchAndAck(t, srv, batch)
	if !second.GetAccepted() || second.GetStatus() != dataplanev1.DataAck_STATUS_DUPLICATE || second.GetReasonCode() != "duplicate" || second.GetContractVersion() != "dataplane.v1" {
		t.Fatalf("retry ack = %#v, want duplicate idempotent retry", second)
	}

	rec := get(t, handler, "/api/v1/events?label=scenario=apt-fileless-c2")
	if got := strings.Count(rec.Body.String(), `"id":"ev-retry"`); got != 1 {
		t.Fatalf("event count = %d, want 1: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/signals?label=scenario=apt-fileless-c2&layer=endpoint")
	if got := strings.Count(rec.Body.String(), `"where":"SIGNAL_WHERE_ENDPOINT"`); got != 3 {
		t.Fatalf("endpoint signal count = %d, want 3: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/signals?label=scenario=apt-fileless-c2&layer=cloud")
	if got := strings.Count(rec.Body.String(), `"where":"SIGNAL_WHERE_CLOUD"`); got != 2 {
		t.Fatalf("cloud signal count = %d, want 2: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/incidents?label=scenario=apt-fileless-c2")
	if got := strings.Count(rec.Body.String(), `"id":"inc-`); got != 1 {
		t.Fatalf("incident count = %d, want 1: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/metrics")
	for _, want := range []string{
		`"data_batches_appended":1`,
		`"events_ingested":1`,
		`"endpoint_signals_ingested":3`,
		`"cloud_signals_emitted":2`,
		`"signals_emitted":5`,
		`"incidents_created":1`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("metrics missing %s after retry: %s", want, rec.Body.String())
		}
	}
}

func TestUploadUpdatesRarityBaselineWithoutDuplicateAmplification(t *testing.T) {
	st := &store.Store{}
	srv := newTestServer(st)
	handler := srv.Handler()
	batch := httpDataBatch("rarity-batch-1", "agent-rarity", "host-rarity", nil, []*signalv1.Signal{{
		Id:           "sig-rarity-download",
		Name:         "download_by_lolbin",
		Where:        signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		BaseRisk:     50,
		GlobalRarity: 1,
		Labels:       labelsForScenario("rarity-append"),
		Entities: []*signalv1.EntityRef{{
			Kind: "container",
			Key:  "checkout-api",
		}},
	}})
	appendBatch(t, srv, batch)
	if got := st.RarityBaselineSnapshot().Count("container:checkout-api", "download_by_lolbin"); got != 1 {
		t.Fatalf("workload baseline count = %d, want 1", got)
	}
	appendBatch(t, srv, batch)
	if got := st.RarityBaselineSnapshot().Count("container:checkout-api", "download_by_lolbin"); got != 1 {
		t.Fatalf("workload baseline count after duplicate = %d, want 1", got)
	}
	rec := get(t, handler, "/api/v1/rarity-baseline?workload=container:checkout-api&signal=download_by_lolbin")
	for _, want := range []string{
		`"count":1`,
		`"container:checkout-api":{"download_by_lolbin":1}`,
		`"global":{"download_by_lolbin":1}`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("rarity baseline response missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestUploadIndexesSecurityDocuments(t *testing.T) {
	st, _ := store.Open("")
	indexer := &recordingIndexer{}
	_, err := ingestworker.NewProcessor(st, indexer).Process(context.Background(), httpDataBatch("batch-index", "agent-index", "host-index", []*eventv1.CanonicalEvent{{Id: "ev-index", Labels: labelsForScenario("apt-fileless-c2"), Behavior: "process.exec"}}, []*signalv1.Signal{
		endpointSignal("web_runtime_spawns_shell", "lin-index", false, processEntity("p-web")),
		endpointSignal("payload_dropped", "lin-index", false, fileEntity("/dev/shm/x.sh")),
		endpointSignal("reverse_shell_pattern", "lin-index", true, processEntity("p-bash"), socketEntity("10.66.0.99:443")),
	}))
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	indexes := map[string]bool{}
	for _, doc := range indexer.docs {
		indexes[doc.Index] = true
	}
	for _, want := range []string{"sysarmor-events", "sysarmor-signals", "sysarmor-incidents", "sysarmor-incident-timeline", "sysarmor-evidence"} {
		if !indexes[want] {
			t.Fatalf("indexed docs missing %s: %+v", want, indexer.docs)
		}
	}
}

func TestAgentsEventsResetAndRecompute(t *testing.T) {
	st := &store.Store{}
	srv := newTestServer(st)
	handler := srv.Handler()
	batch := httpDataBatch("", "agent-a", "host-a", []*eventv1.CanonicalEvent{{
		Id:       "ev-1",
		Labels:   labelsForScenario("apt-staged-drop"),
		Behavior: "process.exec",
		SubjectProc: &eventv1.ProcessRef{
			StableId: "p1",
			Binary:   "/bin/bash",
		},
		LineageId: "lin-a",
	}}, []*signalv1.Signal{
		endpointSignalForScenario("apt-staged-drop", "payload_dropped", "lin-a", false, fileEntity("/var/lib/app/plugins/helper")),
		endpointSignalForScenario("apt-staged-drop", "suspicious_exec_connect", "lin-b", false, fileEntity("/var/lib/app/plugins/helper"), socketEntity("10.66.0.99:443")),
	})
	appendBatch(t, srv, batch)

	rec := get(t, handler, "/api/v1/agents")
	if !strings.Contains(rec.Body.String(), "agent-a") {
		t.Fatalf("agents response missing agent-a: %s", rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/events?label=scenario=apt-staged-drop&behavior=process.exec")
	if !strings.Contains(rec.Body.String(), "ev-1") {
		t.Fatalf("events response missing ev-1: %s", rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/recompute?label=scenario=apt-staged-drop&disable=cloud.cross_lineage")
	if strings.Contains(rec.Body.String(), `"inc-`) {
		t.Fatalf("disabled cross-lineage recompute should not incident: %s", rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/reset?label=scenario=apt-staged-drop", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/events?label=scenario=apt-staged-drop")
	if rec.Body.String() != "[]\n" {
		t.Fatalf("events after reset = %s, want empty list", rec.Body.String())
	}
}

func TestAgentHealthIngestAndQuery(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	health := agenthealth.AgentHealth{
		AgentID:          "agent-a",
		HostID:           "host-a",
		TenantID:         "default",
		Scope:            agenthealth.RuntimeScope{Type: "container", Selector: "abc123"},
		Status:           "ok",
		UptimeSeconds:    12,
		ObservedAt:       time.Now().UTC(),
		Capability:       agenthealth.SensorCapability{Backend: "fake", Version: "dev", SupportsExec: true, SupportsHealth: true, KernelRelease: "test-kernel", BTFAvailable: true, BPFFSAvailable: true},
		Sensor:           agenthealth.SensorHealth{Backend: "fake", Running: true, PolicyLoaded: true, EventsSeen: 3},
		TelemetryBatcher: agenthealth.TelemetryBatcherHealth{QueuedBatches: 1, QueueCapacity: 8},
		TelemetrySender:  agenthealth.TelemetrySenderHealth{SentBatches: 2},
	}
	data, err := json.Marshal(health)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-health", strings.NewReader(string(data)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/agent-health?agent_id=agent-a&tenant_id=default")
	for _, want := range []string{`"agent_id":"agent-a"`, `"scope":{"type":"container","selector":"abc123"}`, `"sensor_capability"`, `"kernel_release":"test-kernel"`, `"sensor_health"`, `"telemetry_batcher_health"`, `"telemetry_sender_health"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("health response missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/agent-health")
	if !strings.Contains(rec.Body.String(), `"agent_id":"agent-a"`) {
		t.Fatalf("health list missing agent-a: %s", rec.Body.String())
	}
	st.AddAgent(store.AgentIdentity{AgentID: "agent-a", HostID: "host-a", TenantID: "default", Version: "test"})
	st.AddAgent(store.AgentIdentity{AgentID: "agent-b", HostID: "host-b", TenantID: "other", Version: "test"})
	st.UpsertAgentHealth(agenthealth.AgentHealth{
		AgentID:    "agent-b",
		HostID:     "host-b",
		TenantID:   "other",
		Scope:      agenthealth.RuntimeScope{Type: "host"},
		Status:     "degraded",
		ObservedAt: time.Now().UTC(),
		Sensor:     agenthealth.SensorHealth{Backend: "fake", Running: false},
	})
	rec = get(t, handler, "/api/v1/agents")
	for _, want := range []string{`"agent_id":"agent-a"`, `"health_status":"ok"`, `"scope":{"type":"container","selector":"abc123"}`, `"sensor_capability":{"backend":"fake","version":"dev","supports_exec":true,"supports_health":true,"kernel_release":"test-kernel","btf_available":true,"bpffs_available":true}`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("agents response missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/agents?tenant_id=default&scope_type=container&health_status=ok")
	if !strings.Contains(rec.Body.String(), `"agent_id":"agent-a"`) || strings.Contains(rec.Body.String(), `"agent_id":"agent-b"`) {
		t.Fatalf("filtered agents response = %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/agents?tenant_id=default&scope_type=container&scope_selector=abc123&health_status=ok")
	if !strings.Contains(rec.Body.String(), `"agent_id":"agent-a"`) || strings.Contains(rec.Body.String(), `"agent_id":"agent-b"`) {
		t.Fatalf("filtered selector agents response = %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/agents?tenant_id=default&scope_type=container&scope_selector=missing&health_status=ok")
	if rec.Body.String() != "[]\n" {
		t.Fatalf("filtered missing selector response = %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/agents?tenant_id=other&scope_type=host&health_status=degraded")
	if !strings.Contains(rec.Body.String(), `"agent_id":"agent-b"`) || strings.Contains(rec.Body.String(), `"agent_id":"agent-a"`) {
		t.Fatalf("filtered other agents response = %s", rec.Body.String())
	}
}

func TestOperatorTokenGuardsHealthWrites(t *testing.T) {
	st := &store.Store{}
	handler := NewServerWithOperatorToken(st, "operator-token").Handler()

	health := agenthealth.AgentHealth{AgentID: "agent-a", HostID: "host-a", TenantID: "default", Status: "ok"}
	healthData, err := json.Marshal(health)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-health", strings.NewReader(string(healthData)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("health without token status = %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/agent-health", strings.NewReader(string(healthData)))
	req.Header.Set("Authorization", "Bearer operator-token")
	req.Header.Set("X-SysArmor-Role", "admin")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health with operator token status = %d body=%s", rec.Code, rec.Body.String())
	}
}
func TestSplitUploadRecomputesScenarioDerivedResults(t *testing.T) {
	st := &store.Store{}
	srv := newTestServer(st)
	handler := srv.Handler()
	scenario := "apt-staged-drop-stream"
	payload := fileEntity("/var/lib/app/plugins/helper")

	appendBatch(t, srv, httpDataBatch("", "", "", nil, []*signalv1.Signal{
		endpointSignalForScenario(scenario, "payload_dropped", "lin-drop", false, payload),
	}))
	rec := get(t, handler, "/api/v1/incidents?label=scenario="+scenario)
	if strings.Contains(rec.Body.String(), `"inc-`) {
		t.Fatalf("first split batch should not create incident: %s", rec.Body.String())
	}

	appendBatch(t, srv, httpDataBatch("", "", "", nil, []*signalv1.Signal{
		endpointSignalForScenario(scenario, "suspicious_exec_connect", "lin-connect", false, payload, socketEntity("10.66.0.99:443")),
	}))

	rec = get(t, handler, "/api/v1/signals?label=scenario="+scenario+"&layer=cloud")
	if got := strings.Count(rec.Body.String(), "dropped_payload_executed_and_connects"); got != 1 {
		t.Fatalf("cloud signal count = %d, want 1: %s", got, rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/incidents?label=scenario="+scenario)
	body := rec.Body.String()
	if got := strings.Count(body, `"inc-`); got != 1 {
		t.Fatalf("incident count = %d, want 1: %s", got, body)
	}
	for _, want := range []string{"lin-drop", "lin-connect"} {
		if !strings.Contains(body, want) {
			t.Fatalf("incident missing lineage %s: %s", want, body)
		}
	}

	appendBatch(t, srv, httpDataBatch("", "", "", []*eventv1.CanonicalEvent{{Id: "noise-1", Labels: labelsForScenario(scenario), Behavior: "process.exec"}}, nil))
	rec = get(t, handler, "/api/v1/signals?label=scenario="+scenario+"&layer=cloud")
	if got := strings.Count(rec.Body.String(), "dropped_payload_executed_and_connects"); got != 1 {
		t.Fatalf("cloud signal duplicated after recompute, count = %d: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/incidents?label=scenario="+scenario)
	if got := strings.Count(rec.Body.String(), `"inc-`); got != 1 {
		t.Fatalf("incident duplicated after recompute, count = %d: %s", got, rec.Body.String())
	}
}

func TestPolicyAPIAssignmentAndCloudRuleDisable(t *testing.T) {
	st := &store.Store{}
	srv := NewServer(st)
	handler := srv.Handler()

	rec := get(t, handler, "/api/v1/rules?where=cloud")
	if !strings.Contains(rec.Body.String(), "dropped_payload_executed_and_connects") {
		t.Fatalf("rules response missing cloud rule: %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/policies?tenant_id=default")
	if !strings.Contains(rec.Body.String(), policymodel.DefaultPolicyID) {
		t.Fatalf("policies response missing default policy: %s", rec.Body.String())
	}

	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "no-cross-incident"
	policy.Version = 1
	policy.CloudRules = []string{"web_shell_chain"}
	policyData, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies?actor=tester&reason=draft", strings.NewReader(string(policyData)))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy post status = %d body=%s", rec.Code, rec.Body.String())
	}

	assignment := policymodel.Assignment{
		TenantID: "default",
		AgentID:  "agent-policy",
		PolicyID: "no-cross-incident",
	}
	assignmentData, err := json.Marshal(assignment)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(string(assignmentData)))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assignment post status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/effective-policy?tenant_id=default&agent_id=agent-policy")
	if !strings.Contains(rec.Body.String(), `"policy_id":"no-cross-incident"`) || strings.Contains(rec.Body.String(), "dropped_payload_executed_and_connects") {
		t.Fatalf("effective policy response = %s", rec.Body.String())
	}

	appendBatch(t, srv, httpDataBatch("", "agent-policy", "host-a", nil, []*signalv1.Signal{
		endpointSignalForScenario("apt-staged-drop-policy", "payload_dropped", "lin-drop", false, fileEntity("/var/lib/app/plugins/helper")),
		endpointSignalForScenario("apt-staged-drop-policy", "suspicious_exec_connect", "lin-connect", false, fileEntity("/var/lib/app/plugins/helper"), socketEntity("10.66.0.99:443")),
	}))
	rec = get(t, handler, "/api/v1/signals?label=scenario=apt-staged-drop-policy&layer=cloud")
	if strings.Contains(rec.Body.String(), "dropped_payload_executed_and_connects") {
		t.Fatalf("disabled cloud rule still emitted signal: %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/incidents?label=scenario=apt-staged-drop-policy")
	if strings.Contains(rec.Body.String(), `"inc-`) {
		t.Fatalf("disabled cloud rule still created incident: %s", rec.Body.String())
	}
}

func TestPolicyAPIDraftRequiresPublishBeforeAssignment(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "draft-policy"
	policy.Version = 2
	policy.Published = false
	policyData, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies?actor=tester&reason=draft", strings.NewReader(string(policyData)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy post status = %d body=%s", rec.Code, rec.Body.String())
	}
	assignmentData := `{"tenant_id":"default","agent_id":"agent-draft","policy_id":"draft-policy","policy_version":2,"actor":"operator","reason":"deploy"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(assignmentData))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("draft assignment status = %d body=%s", rec.Code, rec.Body.String())
	}
	publish := `{"tenant_id":"default","policy_id":"draft-policy","version":2,"published":true,"actor":"reviewer","reason":"ready"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policy-publish", strings.NewReader(publish))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"published":true`) {
		t.Fatalf("publish status = %d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(assignmentData))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("published assignment status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/effective-policy?tenant_id=default&agent_id=agent-draft")
	if !strings.Contains(rec.Body.String(), `"policy_id":"draft-policy"`) || !strings.Contains(rec.Body.String(), `"published":true`) {
		t.Fatalf("effective policy response = %s", rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/policy-audit?tenant_id=default&policy_id=draft-policy")
	for _, want := range []string{`"action":"policy.upsert"`, `"actor":"tester"`, `"action":"policy.publish"`, `"actor":"reviewer"`, `"action":"policy.assign"`, `"actor":"operator"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("policy audit missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestOperatorTokenGuardsControlPlaneWritesAndActorHeader(t *testing.T) {
	st := &store.Store{}
	handler := NewServerWithOperatorToken(st, "operator-token").Handler()
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "guarded-policy"
	policy.Version = 3
	policy.Published = false
	policyData, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies?reason=header-actor", strings.NewReader(string(policyData)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("policy write without operator token status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies?reason=header-actor", strings.NewReader(string(policyData)))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "responder")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("policy write with wrong operator role status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies?reason=header-actor", strings.NewReader(string(policyData)))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "policy_admin")
	req.Header.Set("X-SysArmor-Actor", "header-analyst")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy write with operator token status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/policy-audit?tenant_id=default&policy_id=guarded-policy")
	for _, want := range []string{`"action":"policy.upsert"`, `"actor":"header-analyst"`, `"reason":"header-actor"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("policy audit missing %s: %s", want, rec.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/responses", strings.NewReader(`{"tenant_id":"default","agent_id":"agent-a","action":"collect"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("response write without operator token status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/responses", strings.NewReader(`{"tenant_id":"default","agent_id":"agent-a","action":"collect"}`))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "policy_admin")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("response write with wrong operator role status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/responses", strings.NewReader(`{"tenant_id":"default","agent_id":"agent-a","action":"collect"}`))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "responder")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("response write with responder role status = %d body=%s", rec.Code, rec.Body.String())
	}

	adminPolicy := policymodel.DefaultPolicy("default")
	adminPolicy.PolicyID = "admin-policy"
	adminPolicy.Version = 1
	adminData, err := json.Marshal(adminPolicy)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies", strings.NewReader(string(adminData)))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "admin")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy write with admin role status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestControlCommandsAPICreatesAuditableDownlink(t *testing.T) {
	st := &store.Store{}
	handler := NewServerWithOperatorToken(st, "operator-token").Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"command_id":"ctrl-content-api",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"type":"content_update",
		"payload_json":{"api_version":"sysarmor.content/v1","kind":"iocpack","metadata":{"id":"ioc:test","version":"v1"},"spec":{"value_type":"ip","values":["10.0.0.1"]}},
		"reason":"refresh ioc"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("control command without operator token status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"command_id":"ctrl-content-api",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"type":"content_update",
		"payload_json":{"api_version":"sysarmor.content/v1","kind":"iocpack","metadata":{"id":"ioc:test","version":"v1"},"spec":{"value_type":"ip","values":["10.0.0.1"]}},
		"reason":"refresh ioc"
	}`))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "control_admin")
	req.Header.Set("X-SysArmor-Actor", "control-operator")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"command_id":"ctrl-content-api"`) || !strings.Contains(rec.Body.String(), `"actor":"control-operator"`) {
		t.Fatalf("control command create status = %d body=%s", rec.Code, rec.Body.String())
	}

	got := st.PendingControlCommands("default", "agent-a")
	if len(got) != 1 || got[0].Type != controlmodel.ControlCommandTypeContentUpdate || got[0].Reason != "refresh ioc" || got[0].ContentRef != "ioc:test" || got[0].ContentKind != "iocpack" || got[0].ContentVersion != "v1" {
		t.Fatalf("pending control commands = %+v", got)
	}
	rec = get(t, handler, "/api/v1/control-commands?tenant_id=default&agent_id=agent-a&type=content_update")
	if !strings.Contains(rec.Body.String(), `"status":"pending"`) || !strings.Contains(rec.Body.String(), `"reason":"refresh ioc"`) || !strings.Contains(rec.Body.String(), `"content_ref":"ioc:test"`) {
		t.Fatalf("control command audit response = %s", rec.Body.String())
	}
}

func TestPolicyAssignmentDownlinkCreatesPolicyUpdateCommand(t *testing.T) {
	st := &store.Store{}
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "downlink-policy"
	policy.Version = 7
	policy.Published = true
	st.UpsertPolicy(policy)
	handler := NewServer(st).Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(`{
		"tenant_id":"default",
		"agent_id":"agent-downlink",
		"policy_id":"downlink-policy",
		"policy_version":7,
		"downlink":true,
		"command_id":"ctrl-policy-downlink",
		"actor":"policy-operator",
		"reason":"deploy immediately"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assignment downlink status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"assignment"`, `"control_command"`, `"command_id":"ctrl-policy-downlink"`, `"type":"policy_update"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("assignment downlink response missing %s: %s", want, rec.Body.String())
		}
	}
	commands := st.PendingControlCommands("default", "agent-downlink")
	if len(commands) != 1 || commands[0].CommandID != "ctrl-policy-downlink" || commands[0].PolicyID != "downlink-policy" || commands[0].PolicyVersion != 7 || commands[0].Actor != "policy-operator" {
		t.Fatalf("pending commands = %+v", commands)
	}
}

func TestPolicyAssignmentDownlinkRequiresControlAdmin(t *testing.T) {
	st := &store.Store{}
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "downlink-auth-policy"
	policy.Version = 1
	policy.Published = true
	st.UpsertPolicy(policy)
	handler := NewServerWithOperatorToken(st, "operator-token").Handler()

	body := `{"tenant_id":"default","agent_id":"agent-a","policy_id":"downlink-auth-policy","policy_version":1,"downlink":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(body))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "policy_admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("downlink with policy_admin only status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(body))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "policy_admin,control_admin")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"control_command"`) {
		t.Fatalf("downlink with control_admin status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestControlCommandActionsUpdateLifecycle(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	st.CreateControlCommand(controlmodel.ControlCommand{
		CommandID:   "ctrl-action",
		TenantID:    "default",
		AgentID:     "agent-a",
		Type:        controlmodel.ControlCommandTypeContentUpdate,
		PayloadJSON: []byte(`{"kind":"iocpack"}`),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"action":"cancel",
		"command_id":"ctrl-action",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"actor":"operator",
		"reason":"bad rollout"
	}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"canceled"`) || !strings.Contains(rec.Body.String(), `"error":"bad rollout"`) {
		t.Fatalf("cancel status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := st.PendingControlCommands("default", "agent-a"); len(got) != 0 {
		t.Fatalf("pending after cancel = %+v", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"action":"retry",
		"command_id":"ctrl-action",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"actor":"operator",
		"reason":"retry rollout"
	}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"pending"`) || !strings.Contains(rec.Body.String(), `"reason":"retry rollout"`) {
		t.Fatalf("retry status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := st.PendingControlCommands("default", "agent-a"); len(got) != 1 || got[0].CommandID != "ctrl-action" {
		t.Fatalf("pending after retry = %+v", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/control-commands", strings.NewReader(`{
		"action":"expire",
		"command_id":"ctrl-action",
		"tenant_id":"default",
		"agent_id":"agent-a",
		"reason":"ttl elapsed"
	}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"expired"`) || !strings.Contains(rec.Body.String(), `"error":"ttl elapsed"`) {
		t.Fatalf("expire status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOperatorRoleBindingsAuthorizeControlPlaneWrites(t *testing.T) {
	st := &store.Store{}
	handler := NewServerWithOperatorToken(st, "operator-token").Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/operator-role-bindings", strings.NewReader(`{"actor":"alice","roles":["policy_admin","responder","policy_admin"]}`))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Role", "admin")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"actor":"alice"`) || !strings.Contains(rec.Body.String(), `"roles":["policy_admin","responder"]`) {
		t.Fatalf("role binding upsert status = %d body=%s", rec.Code, rec.Body.String())
	}

	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "bound-policy"
	policy.Version = 1
	policyData, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies", strings.NewReader(string(policyData)))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Actor", "alice")
	req.Header.Set("X-SysArmor-Role", "responder")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy write with bound actor status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/policies", strings.NewReader(string(policyData)))
	req.Header.Set("X-SysArmor-Operator-Token", "operator-token")
	req.Header.Set("X-SysArmor-Actor", "bob")
	req.Header.Set("X-SysArmor-Role", "responder")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("policy write with unbound wrong role status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/operator-role-bindings?actor=alice")
	if !strings.Contains(rec.Body.String(), `"actor":"alice"`) || !strings.Contains(rec.Body.String(), `"policy_admin"`) {
		t.Fatalf("role binding list response = %s", rec.Body.String())
	}
}

func TestResponsePolicyCanRequireApproval(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "approval-policy"
	policy.Version = 4
	policy.Response = responsemodel.Policy{
		AllowedActions:   []string{"collect"},
		AllowedModes:     []string{"observe"},
		ApprovalRequired: true,
	}
	policyData, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies", strings.NewReader(string(policyData)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy post status = %d body=%s", rec.Code, rec.Body.String())
	}
	assignmentData := `{"tenant_id":"default","agent_id":"agent-response-policy","policy_id":"approval-policy","policy_version":4}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(assignmentData))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assignment post status = %d body=%s", rec.Code, rec.Body.String())
	}
	cmd := `{"tenant_id":"default","agent_id":"agent-response-policy","action":"collect","target":"process:p1"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/responses", strings.NewReader(cmd))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("response post status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"policy_id":"approval-policy"`, `"policy_version":4`, `"status":"pending_approval"`, `"approval_required":true`, `"approval_status":"required"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("response policy output missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/responses?tenant_id=default&agent_id=agent-response-policy&pending=true")
	if strings.Contains(rec.Body.String(), `"approval-policy"`) {
		t.Fatalf("pending_approval response should not be pending before approval: %s", rec.Body.String())
	}
}

func TestResponsePolicyCanRequireMultiApprovalRoles(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "multi-approval-policy"
	policy.Version = 5
	policy.Response = responsemodel.Policy{
		AllowedActions:    []string{"collect"},
		AllowedModes:      []string{"observe"},
		ApprovalRequired:  true,
		ApprovalThreshold: 2,
		ApprovalRoles:     []string{"responder", "security_admin"},
	}
	policyData, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies", strings.NewReader(string(policyData)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy post status = %d body=%s", rec.Code, rec.Body.String())
	}
	assignmentData := `{"tenant_id":"default","agent_id":"agent-multi-approval","policy_id":"multi-approval-policy","policy_version":5}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/policy-assignments", strings.NewReader(assignmentData))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assignment post status = %d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/responses", strings.NewReader(`{"response_id":"resp-multi-http","tenant_id":"default","agent_id":"agent-multi-approval","action":"collect","target":"process:p1"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("response post status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"approval_threshold":2`, `"approval_roles":["responder","security_admin"]`, `"status":"pending_approval"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("response policy output missing %s: %s", want, rec.Body.String())
		}
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/response-approvals", strings.NewReader(`{"tenant_id":"default","agent_id":"agent-multi-approval","response_id":"resp-multi-http","approved":true,"actor":"viewer","role":"viewer"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("wrong-role approval status = %d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/response-approvals", strings.NewReader(`{"tenant_id":"default","agent_id":"agent-multi-approval","response_id":"resp-multi-http","approved":true,"actor":"responder-a","role":"responder"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"approval_status":"partial"`) {
		t.Fatalf("first approval status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/responses?tenant_id=default&agent_id=agent-multi-approval&pending=true")
	if strings.Contains(rec.Body.String(), `"resp-multi-http"`) {
		t.Fatalf("partial approval response should not be pending: %s", rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/response-approvals", strings.NewReader(`{"tenant_id":"default","agent_id":"agent-multi-approval","response_id":"resp-multi-http","approved":true,"actor":"security-b","role":"security_admin"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"approval_status":"approved"`) {
		t.Fatalf("second approval status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/responses?tenant_id=default&agent_id=agent-multi-approval&pending=true")
	if !strings.Contains(rec.Body.String(), `"resp-multi-http"`) {
		t.Fatalf("approved response should be pending: %s", rec.Body.String())
	}
}

func appendBatch(t *testing.T, srv *Server, batch *dataplanev1.DataBatch) {
	t.Helper()
	_ = appendBatchAndAck(t, srv, batch)
}

func appendBatchAndAck(t *testing.T, srv *Server, batch *dataplanev1.DataBatch) *dataplanev1.DataAck {
	t.Helper()
	if batch.Header == nil {
		batch.Header = &dataplanev1.BatchHeader{AgentId: "agent-a", HostId: "host-a", TenantId: "default"}
	}
	if batch.Header.AgentId == "" {
		batch.Header.AgentId = "agent-a"
	}
	if batch.Header.HostId == "" {
		batch.Header.HostId = "host-a"
	}
	if batch.Header.TenantId == "" {
		batch.Header.TenantId = "default"
	}
	st, ok := srv.store.(*store.Store)
	if !ok {
		t.Fatalf("test server store type = %T, want *store.Store", srv.store)
	}
	duplicate := isDuplicateTestBatch(st, batch)
	st.RecordDataBatchAppend(store.AgentIdentityFromDataBatch(batch), batch.GetHeader().GetBatchId(), "grpc", time.Now().UTC())
	if err := st.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	acceptedEvents := 0
	acceptedSignals := 0
	if !duplicate {
		result, err := ingestworker.NewProcessor(st, nil).Process(context.Background(), batch)
		if err != nil {
			t.Fatalf("Process() error = %v", err)
		}
		acceptedEvents = result.AcceptedEvents
		acceptedSignals = result.AcceptedSignals
	}
	status := dataplanev1.DataAck_STATUS_ACCEPTED
	message := "accepted"
	if duplicate {
		status = dataplanev1.DataAck_STATUS_DUPLICATE
		message = "duplicate"
	}
	return &dataplanev1.DataAck{
		BatchId:         batch.GetHeader().GetBatchId(),
		Accepted:        true,
		Status:          status,
		Message:         message,
		ReasonCode:      message,
		CommittedCursor: batch.GetHeader().GetBatchId(),
		ServerTime:      time.Now().UTC().Format(time.RFC3339Nano),
		AcceptedEvents:  uint64(acceptedEvents),
		AcceptedSignals: uint64(acceptedSignals),
		ContractVersion: "dataplane.v1",
	}
}

func isDuplicateTestBatch(st *store.Store, batch *dataplanev1.DataBatch) bool {
	header := batch.GetHeader()
	if header.GetBatchId() == "" {
		return false
	}
	for _, session := range st.ListAgentSessions(header.GetTenantId(), header.GetAgentId()) {
		if session.LastAckCursor == header.GetBatchId() {
			return true
		}
	}
	return false
}

func httpDataBatch(batchID, agentID, hostID string, events []*eventv1.CanonicalEvent, signals []*signalv1.Signal) *dataplanev1.DataBatch {
	batch := &dataplanev1.DataBatch{
		Header: &dataplanev1.BatchHeader{BatchId: batchID, AgentId: agentID, HostId: hostID, TenantId: "default"},
	}
	for _, ev := range events {
		batch.Events = append(batch.Events, &dataplanev1.EventFrame{Event: ev})
	}
	for _, sig := range signals {
		batch.Signals = append(batch.Signals, &dataplanev1.SignalFrame{Signal: sig})
	}
	return batch
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%s", path, rec.Code, rec.Body.String())
	}
	return rec
}

func endpointSignal(name, lineage string, terminal bool, entities ...*signalv1.EntityRef) *signalv1.Signal {
	return endpointSignalForScenario("apt-fileless-c2", name, lineage, terminal, entities...)
}

func endpointSignalForScenario(scenario, name, lineage string, terminal bool, entities ...*signalv1.EntityRef) *signalv1.Signal {
	return &signalv1.Signal{
		Name:         name,
		Where:        signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		BaseRisk:     50,
		GlobalRarity: 1,
		LineageId:    lineage,
		Terminal:     terminal,
		Entities:     entities,
		Labels:       labelsForScenario(scenario),
	}
}

func labelsForScenario(scenario string) map[string]string {
	return map[string]string{"scenario": scenario}
}

func processEntity(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "process", Key: key, Role: "subject"}
}

func fileEntity(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "file", Key: "file:" + key, Role: "object"}
}

func socketEntity(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "socket", Key: "socket:" + key, Role: "object"}
}
