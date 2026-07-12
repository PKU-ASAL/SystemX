package managerapi

import (
	"encoding/json"
	"testing"
	"time"

	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func TestUIOverviewReturnsManagerSummary(t *testing.T) {
	st := &store.Store{}
	st.AddAgent(store.AgentIdentity{AgentID: "agent-a", HostID: "host-a", TenantID: "default"})
	st.AddAgent(store.AgentIdentity{AgentID: "agent-b", HostID: "host-b", TenantID: "default"})
	st.AddAgent(store.AgentIdentity{AgentID: "agent-c", HostID: "host-c", TenantID: "default"})
	st.UpsertAgentHealth(agenthealth.AgentHealth{
		AgentID:    "agent-a",
		TenantID:   "default",
		Status:     "ok",
		ObservedAt: time.Now().UTC(),
	})
	st.UpsertAgentHealth(agenthealth.AgentHealth{
		AgentID:    "agent-b",
		TenantID:   "default",
		Status:     "degraded",
		ObservedAt: time.Now().UTC(),
	})
	st.Metrics = store.Metrics{
		EventsIngested: 12,
		SignalsEmitted: 5,
	}
	st.Incidents = []*incidentv1.Incident{
		{Id: "inc-critical", Severity: 95},
		{Id: "inc-high", Severity: 75},
		{Id: "inc-medium", Severity: 45},
	}

	rec := get(t, NewServer(st).Handler(), "/api/v1/ui/overview")
	var got struct {
		Agents struct {
			Total    int `json:"total"`
			Online   int `json:"online"`
			Degraded int `json:"degraded"`
			Offline  int `json:"offline"`
		} `json:"agents"`
		Telemetry struct {
			Events24h  uint64 `json:"events_24h"`
			Signals24h uint64 `json:"signals_24h"`
		} `json:"telemetry"`
		Incidents struct {
			Open     int `json:"open"`
			Critical int `json:"critical"`
			High     int `json:"high"`
			Medium   int `json:"medium"`
		} `json:"incidents"`
		Store struct {
			Backend string `json:"backend"`
		} `json:"store"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode overview: %v body=%s", err, rec.Body.String())
	}
	if got.Agents.Total != 3 || got.Agents.Online != 1 || got.Agents.Degraded != 1 || got.Agents.Offline != 1 {
		t.Fatalf("agents summary = %+v", got.Agents)
	}
	if got.Telemetry.Events24h != 12 || got.Telemetry.Signals24h != 5 {
		t.Fatalf("telemetry summary = %+v", got.Telemetry)
	}
	if got.Incidents.Open != 3 || got.Incidents.Critical != 1 || got.Incidents.High != 1 || got.Incidents.Medium != 1 {
		t.Fatalf("incident summary = %+v", got.Incidents)
	}
	if got.Store.Backend != "memory" {
		t.Fatalf("store backend = %q", got.Store.Backend)
	}
}
