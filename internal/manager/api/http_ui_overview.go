package managerapi

import (
	"net/http"
	"time"
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
	metrics := s.store.MetricsSnapshot()
	info := s.store.Info()
	writeJSON(w, overviewResponse{
		GeneratedAt: time.Now().UTC(),
		Agents:      s.overviewAgents(),
		Telemetry: overviewTelemetry{
			Events24h:  metrics.EventsIngested,
			Signals24h: metrics.SignalsEmitted,
		},
		Incidents: s.overviewIncidents(),
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

func (s *Server) overviewIncidents() overviewIncidents {
	incidents := s.store.ListIncidents(nil)
	summary := overviewIncidents{}

	for _, incident := range incidents {
		if incident.GetStatus() != "closed" && incident.GetStatus() != "contained" {
			summary.Open++
		}
		switch {
		case incident.GetSeverity() >= 90:
			summary.Critical++
		case incident.GetSeverity() >= 70:
			summary.High++
		case incident.GetSeverity() >= 40:
			summary.Medium++
		}
	}

	return summary
}
