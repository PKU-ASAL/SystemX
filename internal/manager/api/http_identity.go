package managerapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	managerauth "github.com/sysarmor/sysarmor-next-project/internal/manager/auth"
)

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"ok": true, "store": s.store.Info()})
}

func (s *Server) reset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireOperator(w, r, "admin") {
		return
	}
	labels := parseLabelSelector(r.URL.Query()["label"])
	s.store.DeleteByLabels(labels)
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	if len(labels) == 0 {
		if err := s.store.ResetMetrics(); err != nil {
			http.Error(w, fmt.Sprintf("reset metrics: %v", err), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]any{"ok": true, "labels": labels})
}

func (s *Server) agents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	tenantID := q.Get("tenant_id")
	scopeType := q.Get("scope_type")
	scopeSelector := q.Get("scope_selector")
	healthStatus := q.Get("health_status")
	agents := s.store.ListAgents()
	out := make([]AgentListItem, 0, len(agents))
	for _, agent := range agents {
		if tenantID != "" && agent.TenantID != tenantID {
			continue
		}
		item := AgentListItem{
			AgentID:      agent.AgentID,
			HostID:       agent.HostID,
			TenantID:     agent.TenantID,
			Version:      agent.Version,
			AuthType:     agent.AuthType,
			CertIdentity: agent.CertIdentity,
		}
		if health, ok := s.store.GetAgentHealth(agent.TenantID, agent.AgentID); ok {
			item.HealthStatus = health.Status
			item.Scope = health.Scope
			item.Capability = health.Capability
			item.HealthObserved = health.ObservedAt
		}
		if scopeType != "" && item.Scope.Type != scopeType {
			continue
		}
		if scopeSelector != "" && item.Scope.Selector != scopeSelector {
			continue
		}
		if healthStatus != "" && item.HealthStatus != healthStatus {
			continue
		}
		out = append(out, item)
	}
	writeJSON(w, out)
}

func (s *Server) agentHealth(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if !s.requireOperator(w, r, "admin") {
			return
		}
		var health agenthealth.AgentHealth
		if err := json.NewDecoder(r.Body).Decode(&health); err != nil {
			http.Error(w, fmt.Sprintf("decode agent health: %v", err), http.StatusBadRequest)
			return
		}
		if health.AgentID == "" {
			http.Error(w, "agent_id is required", http.StatusBadRequest)
			return
		}
		s.store.UpsertAgentHealth(health)
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	case http.MethodGet:
		q := r.URL.Query()
		agentID := q.Get("agent_id")
		if agentID == "" {
			writeJSON(w, s.store.ListAgentHealth())
			return
		}
		health, ok := s.store.GetAgentHealth(q.Get("tenant_id"), agentID)
		if !ok {
			http.Error(w, "agent health not found", http.StatusNotFound)
			return
		}
		writeJSON(w, health)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) requireOperator(w http.ResponseWriter, r *http.Request, roles ...string) bool {
	if principal, ok := managerauth.PrincipalFromContext(r.Context()); ok {
		requiredRole := "operator"
		for _, role := range roles {
			switch role {
			case "admin", "policy_admin", "control_admin", "incident_admin":
				requiredRole = "admin"
			}
		}
		if principal.HasRole(requiredRole) {
			return true
		}
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func (s *Server) actorFromRequest(r *http.Request, _ string) string {
	if principal, ok := managerauth.PrincipalFromContext(r.Context()); ok {
		return principal.Subject
	}
	return ""
}

func (s *Server) roleFromRequest(r *http.Request, _ string) string {
	if principal, ok := managerauth.PrincipalFromContext(r.Context()); ok && len(principal.Roles) > 0 {
		return principal.Roles[0]
	}
	return ""
}

func (s *Server) agentSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	writeJSON(w, map[string]any{"sessions": s.store.ListAgentSessions(q.Get("tenant_id"), q.Get("agent_id"))})
}

func (s *Server) dataResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	agentID := q.Get("agent_id")
	if agentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	tenantID := q.Get("tenant_id")
	if tenantID == "" {
		tenantID = "default"
	}
	writeJSON(w, s.resumeCursor(tenantID, agentID))
}

func (s *Server) resumeCursor(tenantID, agentID string) DataResume {
	resume := DataResume{TenantID: tenantID, AgentID: agentID}
	sessions := s.store.ListAgentSessions(tenantID, agentID)
	if len(sessions) > 0 {
		resume.SessionID = sessions[0].SessionID
		resume.ResumeCursor = sessions[0].LastAckCursor
	}
	return resume
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.MetricsSnapshot())
}

func (s *Server) storeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.store.Info())
}

func (s *Server) rarityBaseline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	baseline := s.store.RarityBaselineSnapshot()
	writeJSON(w, map[string]any{
		"baseline": baseline,
		"count":    baseline.Count(q.Get("workload"), q.Get("signal")),
	})
}
