package managerapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	platformopensearch "github.com/sysarmor/sysarmor-next-project/internal/platform/opensearch"
	"google.golang.org/protobuf/encoding/protojson"
)

type overviewResponse struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Agents      overviewAgentsSummary `json:"agents"`
	Telemetry   overviewTelemetry     `json:"telemetry"`
	Incidents   overviewIncidents     `json:"incidents"`
	Store       overviewStore         `json:"store"`
}

type overviewAgentsSummary struct {
	Total    int `json:"total"`
	Online   int `json:"online"`
	Degraded int `json:"degraded"`
	Offline  int `json:"offline"`
}

type overviewTelemetry struct {
	Events24h  uint64 `json:"events_24h"`
	Signals24h uint64 `json:"signals_24h"`
}

type overviewIncidents struct {
	Open     int `json:"open"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
}

type overviewStore struct {
	Backend               string `json:"backend"`
	PostgresSchemaVersion int    `json:"postgres_schema_version,omitempty"`
}

func (s *Server) uiOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		http.Error(w, "tenant_id is required", http.StatusBadRequest)
		return
	}
	incidents, err := s.overviewIncidents(r.Context(), tenantID)
	if err != nil {
		http.Error(w, fmt.Sprintf("query incident reports: %v", err), http.StatusBadGateway)
		return
	}
	metrics := s.store.MetricsSnapshot()
	info := s.store.Info()
	writeJSON(w, overviewResponse{
		GeneratedAt: time.Now().UTC(),
		Agents:      s.overviewAgents(),
		Telemetry: overviewTelemetry{
			Events24h:  metrics.EventsIngested,
			Signals24h: metrics.SignalsEmitted,
		},
		Incidents: incidents,
		Store: overviewStore{
			Backend:               info.Backend,
			PostgresSchemaVersion: info.PostgresSchema,
		},
	})
}

func (s *Server) overviewAgents() overviewAgentsSummary {
	agents := s.store.ListAgents()
	summary := overviewAgentsSummary{Total: len(agents)}

	for _, agent := range agents {
		health, ok := s.store.GetAgentHealth(agent.TenantID, agent.AgentID)
		if !ok {
			summary.Offline++
			continue
		}
		switch health.Status {
		case "ok", "healthy":
			summary.Online++
		case "degraded":
			summary.Degraded++
		default:
			summary.Offline++
		}
	}

	return summary
}

func (s *Server) overviewIncidents(ctx context.Context, tenantID string) (overviewIncidents, error) {
	summary := overviewIncidents{}
	if s.searcher == nil {
		return summary, nil
	}
	raw, err := s.searchTelemetry(ctx, platformopensearch.SearchRequest{Index: "sysarmor-incidents", Size: 1000, Labels: map[string]string{"tenant_id": tenantID}})
	if err != nil {
		return summary, err
	}
	for _, document := range raw {
		incident := &incidentv1.Incident{}
		if err := protojson.Unmarshal(document, incident); err != nil {
			return summary, err
		}
		summary.Open++
		switch {
		case incident.GetSeverity() >= 90:
			summary.Critical++
		case incident.GetSeverity() >= 70:
			summary.High++
		case incident.GetSeverity() >= 40:
			summary.Medium++
		}
	}
	return summary, nil
}
