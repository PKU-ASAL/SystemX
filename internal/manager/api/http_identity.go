package managerapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
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

func (s *Server) operatorAuthorized(r *http.Request) bool {
	if s.operatorToken == "" {
		return true
	}
	if r.Header.Get("X-SysArmor-Operator-Token") == s.operatorToken {
		return true
	}
	if r.Header.Get("Authorization") == "Bearer "+s.operatorToken {
		return true
	}
	return false
}

func (s *Server) operatorAuthorizedFor(r *http.Request, roles ...string) bool {
	if !s.operatorAuthorized(r) {
		return false
	}
	if s.operatorToken == "" || len(roles) == 0 {
		return true
	}
	if boundRoles, ok := s.store.OperatorRolesForActor(s.actorFromRequest(r, "")); ok {
		return rolesAllowed(boundRoles, roles...)
	}
	for _, role := range strings.Split(r.Header.Get("X-SysArmor-Role"), ",") {
		if rolesAllowed([]string{role}, roles...) {
			return true
		}
	}
	return false
}

func (s *Server) requireOperator(w http.ResponseWriter, r *http.Request, roles ...string) bool {
	if s.operatorToken == "" {
		return true
	}
	if !s.operatorAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if !s.operatorAuthorizedFor(r, roles...) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) actorFromRequest(r *http.Request, explicit string) string {
	if explicit != "" {
		return explicit
	}
	return r.Header.Get("X-SysArmor-Actor")
}

func (s *Server) roleFromRequest(r *http.Request, explicit string) string {
	if explicit != "" {
		return explicit
	}
	if roles, ok := s.store.OperatorRolesForActor(s.actorFromRequest(r, "")); ok && len(roles) > 0 {
		return roles[0]
	}
	for _, role := range strings.Split(r.Header.Get("X-SysArmor-Role"), ",") {
		role = strings.TrimSpace(role)
		if role != "" {
			return role
		}
	}
	return ""
}

func rolesAllowed(granted []string, required ...string) bool {
	for _, role := range granted {
		role = strings.TrimSpace(role)
		if role == "admin" {
			return true
		}
		for _, allowed := range required {
			if role == allowed {
				return true
			}
		}
	}
	return false
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

func (s *Server) operatorRoleBindings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"bindings": s.store.ListOperatorRoleBindings(r.URL.Query().Get("actor"))})
	case http.MethodPost:
		if !s.requireOperator(w, r, "admin") {
			return
		}
		var req operatorRoleBindingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode operator role binding: %v", err), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Actor) == "" {
			http.Error(w, "actor is required", http.StatusBadRequest)
			return
		}
		binding := s.store.UpsertOperatorRoleBinding(store.OperatorRoleBinding{Actor: req.Actor, Roles: req.Roles})
		writeJSON(w, binding)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
