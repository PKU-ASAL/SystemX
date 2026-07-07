package managerapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

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
