package link1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestUploadTriggersAnalyticsAndQueries(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	batch := &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", TenantId: "default"},
		Signals: []*signalv1.Signal{
			endpointSignal("web_runtime_spawns_shell", "lin-a", false, processEntity("p-web")),
			endpointSignal("payload_dropped", "lin-a", false, fileEntity("/dev/shm/x.sh")),
			endpointSignal("reverse_shell_pattern", "lin-a", true, processEntity("p-bash"), socketEntity("10.66.0.99:443")),
		},
	}
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", strings.NewReader(string(data)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/signals?scenario=apt-fileless-c2&layer=cloud", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("signals status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "web_shell_chain") {
		t.Fatalf("cloud signals missing web_shell_chain: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/incidents?scenario=apt-fileless-c2", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("incidents status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "rarity+causal-topk") {
		t.Fatalf("incident missing converge method: %s", rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/metrics")
	for _, want := range []string{
		`"upload_batches":1`,
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

func TestHTTPUploadAckIncludesBatchID(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	batch := &analyticsv1.UploadBatch{
		BatchId: "00000000000000000042",
		Agent:   &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", TenantId: "default"},
		Events: []*eventv1.CanonicalEvent{{
			Id:   "ev-ack",
			Kind: eventv1.EventKind_EVENT_KIND_EXEC,
		}},
	}
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", strings.NewReader(string(data)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d body=%s", rec.Code, rec.Body.String())
	}
	ack := &analyticsv1.UploadAck{}
	if err := protojson.Unmarshal(rec.Body.Bytes(), ack); err != nil {
		t.Fatalf("decode ack: %v body=%s", err, rec.Body.String())
	}
	if !ack.GetOk() || ack.GetBatchId() != batch.GetBatchId() || ack.GetAcceptedEvents() != 1 {
		t.Fatalf("ack = %#v", ack)
	}
}

func TestHTTPUploadRetryIsIdempotentForAcceptedCounts(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	batch := &analyticsv1.UploadBatch{
		BatchId: "00000000000000000007",
		Agent:   &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", TenantId: "default"},
		Events: []*eventv1.CanonicalEvent{{
			Id:       "ev-retry",
			Scenario: "apt-fileless-c2",
			Kind:     eventv1.EventKind_EVENT_KIND_EXEC,
		}},
		Signals: []*signalv1.Signal{
			endpointSignal("web_runtime_spawns_shell", "lin-retry", false, processEntity("p-web")),
			endpointSignal("payload_dropped", "lin-retry", false, fileEntity("/dev/shm/x.sh")),
			endpointSignal("reverse_shell_pattern", "lin-retry", true, processEntity("p-bash"), socketEntity("10.66.0.99:443")),
		},
	}
	first := uploadAndAck(t, handler, batch)
	if first.GetAcceptedEvents() != 1 || first.GetAcceptedSignals() != 3 {
		t.Fatalf("first ack = %#v", first)
	}
	second := uploadAndAck(t, handler, batch)
	if second.GetAcceptedEvents() != 0 || second.GetAcceptedSignals() != 0 {
		t.Fatalf("retry ack = %#v, want zero newly accepted records", second)
	}

	rec := get(t, handler, "/api/v1/events?scenario=apt-fileless-c2")
	if got := strings.Count(rec.Body.String(), `"id":"ev-retry"`); got != 1 {
		t.Fatalf("event count = %d, want 1: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/signals?scenario=apt-fileless-c2&layer=endpoint")
	if got := strings.Count(rec.Body.String(), `"where":"SIGNAL_WHERE_ENDPOINT"`); got != 3 {
		t.Fatalf("endpoint signal count = %d, want 3: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/signals?scenario=apt-fileless-c2&layer=cloud")
	if got := strings.Count(rec.Body.String(), `"where":"SIGNAL_WHERE_CLOUD"`); got != 2 {
		t.Fatalf("cloud signal count = %d, want 2: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/incidents?scenario=apt-fileless-c2")
	if got := strings.Count(rec.Body.String(), `"id":"inc-`); got != 1 {
		t.Fatalf("incident count = %d, want 1: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/metrics")
	for _, want := range []string{
		`"upload_batches":2`,
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

func TestHTTPUploadRequiresAgentIdentity(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", strings.NewReader(string(data)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "tenant_id") {
		t.Fatalf("upload status = %d body=%s, want missing tenant_id bad request", rec.Code, rec.Body.String())
	}
}

func TestAgentsEventsResetAndRecompute(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	batch := &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", TenantId: "default", Version: "test"},
		Events: []*eventv1.CanonicalEvent{{
			Id:       "ev-1",
			Scenario: "apt-staged-drop",
			Kind:     eventv1.EventKind_EVENT_KIND_EXEC,
			SubjectProc: &eventv1.ProcessRef{
				StableId: "p1",
				Binary:   "/bin/bash",
			},
			LineageId: "lin-a",
		}},
		Signals: []*signalv1.Signal{
			endpointSignalForScenario("apt-staged-drop", "payload_dropped", "lin-a", false, fileEntity("/var/lib/app/plugins/helper")),
			endpointSignalForScenario("apt-staged-drop", "suspicious_exec_connect", "lin-b", false, fileEntity("/var/lib/app/plugins/helper"), socketEntity("10.66.0.99:443")),
		},
	}
	upload(t, handler, batch)

	rec := get(t, handler, "/api/v1/agents")
	if !strings.Contains(rec.Body.String(), "agent-a") {
		t.Fatalf("agents response missing agent-a: %s", rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/events?scenario=apt-staged-drop&kind=EXEC")
	if !strings.Contains(rec.Body.String(), "ev-1") {
		t.Fatalf("events response missing ev-1: %s", rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/recompute?scenario=apt-staged-drop&disable=cloud.cross_lineage")
	if strings.Contains(rec.Body.String(), `"inc-`) {
		t.Fatalf("disabled cross-lineage recompute should not incident: %s", rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/reset?scenario=apt-staged-drop", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/events?scenario=apt-staged-drop")
	if rec.Body.String() != "[]\n" {
		t.Fatalf("events after reset = %s, want empty list", rec.Body.String())
	}
}

func TestAgentHealthIngestAndQuery(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	health := agenthealth.AgentHealth{
		AgentID:       "agent-a",
		HostID:        "host-a",
		TenantID:      "default",
		Scope:         agenthealth.RuntimeScope{Type: "container", Selector: "abc123"},
		Status:        "ok",
		UptimeSeconds: 12,
		ObservedAt:    time.Now().UTC(),
		Sensor:        agenthealth.SensorHealth{Backend: "fake", Running: true, PolicyLoaded: true, EventsSeen: 3},
		Queue:         agenthealth.QueueHealth{QueuedBatches: 1, QueuedBytes: 256},
		Upload:        agenthealth.UploadHealth{RemainingBatches: 1},
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
	for _, want := range []string{`"agent_id":"agent-a"`, `"scope":{"type":"container","selector":"abc123"}`, `"sensor_health"`, `"queue_health"`, `"upload_health"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("health response missing %s: %s", want, rec.Body.String())
		}
	}
	rec = get(t, handler, "/api/v1/agent-health")
	if !strings.Contains(rec.Body.String(), `"agent_id":"agent-a"`) {
		t.Fatalf("health list missing agent-a: %s", rec.Body.String())
	}
	st.AddAgent(&analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", TenantId: "default", Version: "test"})
	rec = get(t, handler, "/api/v1/agents")
	for _, want := range []string{`"agent_id":"agent-a"`, `"health_status":"ok"`, `"scope":{"type":"container","selector":"abc123"}`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("agents response missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestHTTPAuthRequiresDevTokenForUploadAndHealth(t *testing.T) {
	st := &store.Store{}
	handler := NewServerWithAuth(st, "dev-token").Handler()
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", TenantId: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", strings.NewReader(string(data)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("upload without token status = %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/upload", strings.NewReader(string(data)))
	req.Header.Set("X-SysArmor-Agent-Token", "dev-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload with token status = %d body=%s", rec.Code, rec.Body.String())
	}

	health := agenthealth.AgentHealth{AgentID: "agent-a", HostID: "host-a", TenantID: "default", Status: "ok"}
	healthData, err := json.Marshal(health)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/agent-health", strings.NewReader(string(healthData)))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("health without token status = %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/agent-health", strings.NewReader(string(healthData)))
	req.Header.Set("Authorization", "Bearer dev-token")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health with token status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSplitUploadRecomputesScenarioDerivedResults(t *testing.T) {
	st := &store.Store{}
	handler := NewServer(st).Handler()
	scenario := "apt-staged-drop-stream"
	payload := fileEntity("/var/lib/app/plugins/helper")

	upload(t, handler, &analyticsv1.UploadBatch{
		Signals: []*signalv1.Signal{
			endpointSignalForScenario(scenario, "payload_dropped", "lin-drop", false, payload),
		},
	})
	rec := get(t, handler, "/api/v1/incidents?scenario="+scenario)
	if strings.Contains(rec.Body.String(), `"inc-`) {
		t.Fatalf("first split batch should not create incident: %s", rec.Body.String())
	}

	upload(t, handler, &analyticsv1.UploadBatch{
		Signals: []*signalv1.Signal{
			endpointSignalForScenario(scenario, "suspicious_exec_connect", "lin-connect", false, payload, socketEntity("10.66.0.99:443")),
		},
	})

	rec = get(t, handler, "/api/v1/signals?scenario="+scenario+"&layer=cloud")
	if got := strings.Count(rec.Body.String(), "dropped_payload_executed_and_connects"); got != 1 {
		t.Fatalf("cloud signal count = %d, want 1: %s", got, rec.Body.String())
	}

	rec = get(t, handler, "/api/v1/incidents?scenario="+scenario)
	body := rec.Body.String()
	if got := strings.Count(body, `"inc-`); got != 1 {
		t.Fatalf("incident count = %d, want 1: %s", got, body)
	}
	for _, want := range []string{"lin-drop", "lin-connect"} {
		if !strings.Contains(body, want) {
			t.Fatalf("incident missing lineage %s: %s", want, body)
		}
	}

	upload(t, handler, &analyticsv1.UploadBatch{
		Events: []*eventv1.CanonicalEvent{{Id: "noise-1", Scenario: scenario, Kind: eventv1.EventKind_EVENT_KIND_EXEC}},
	})
	rec = get(t, handler, "/api/v1/signals?scenario="+scenario+"&layer=cloud")
	if got := strings.Count(rec.Body.String(), "dropped_payload_executed_and_connects"); got != 1 {
		t.Fatalf("cloud signal duplicated after recompute, count = %d: %s", got, rec.Body.String())
	}
	rec = get(t, handler, "/api/v1/incidents?scenario="+scenario)
	if got := strings.Count(rec.Body.String(), `"inc-`); got != 1 {
		t.Fatalf("incident duplicated after recompute, count = %d: %s", got, rec.Body.String())
	}
}

func upload(t *testing.T, handler http.Handler, batch *analyticsv1.UploadBatch) {
	t.Helper()
	_ = uploadAndAck(t, handler, batch)
}

func uploadAndAck(t *testing.T, handler http.Handler, batch *analyticsv1.UploadBatch) *analyticsv1.UploadAck {
	t.Helper()
	if batch.Agent == nil {
		batch.Agent = &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", TenantId: "default"}
	}
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upload", strings.NewReader(string(data)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d body=%s", rec.Code, rec.Body.String())
	}
	ack := &analyticsv1.UploadAck{}
	if err := protojson.Unmarshal(rec.Body.Bytes(), ack); err != nil {
		t.Fatalf("decode ack: %v body=%s", err, rec.Body.String())
	}
	return ack
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
		Scenario:     scenario,
	}
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
