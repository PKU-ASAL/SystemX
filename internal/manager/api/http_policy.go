package managerapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
)

func (s *Server) rules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.store.ListRules(r.URL.Query().Get("where")))
}

func (s *Server) policies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		if policyID := q.Get("policy_id"); policyID != "" {
			version := parseUint(q.Get("version"))
			policy, ok := s.store.GetPolicy(q.Get("tenant_id"), policyID, version)
			if !ok {
				http.Error(w, "policy not found", http.StatusNotFound)
				return
			}
			writeJSON(w, policy)
			return
		}
		writeJSON(w, s.store.ListPolicies(q.Get("tenant_id")))
	case http.MethodPost:
		if !s.requireOperator(w, r, "policy_admin") {
			return
		}
		var policy policymodel.Policy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
			http.Error(w, fmt.Sprintf("decode policy: %v", err), http.StatusBadRequest)
			return
		}
		if policy.PolicyID == "" {
			http.Error(w, "policy_id is required", http.StatusBadRequest)
			return
		}
		policy = s.store.UpsertPolicy(policy)
		s.recordPolicyAudit(policymodel.AuditRecord{
			TenantID:      policy.TenantID,
			Action:        "policy.upsert",
			PolicyID:      policy.PolicyID,
			PolicyVersion: policy.Version,
			Actor:         s.actorFromRequest(r, r.URL.Query().Get("actor")),
			Reason:        r.URL.Query().Get("reason"),
		})
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, policy)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) policyPublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireOperator(w, r, "policy_admin") {
		return
	}
	var req policyPublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode policy publish: %v", err), http.StatusBadRequest)
		return
	}
	if req.PolicyID == "" {
		http.Error(w, "policy_id is required", http.StatusBadRequest)
		return
	}
	policy, ok := s.store.PublishPolicy(req.TenantID, req.PolicyID, req.Version, req.Published)
	if !ok {
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	action := "policy.unpublish"
	if req.Published {
		action = "policy.publish"
	}
	s.recordPolicyAudit(policymodel.AuditRecord{
		TenantID:      policy.TenantID,
		Action:        action,
		PolicyID:      policy.PolicyID,
		PolicyVersion: policy.Version,
		Actor:         s.actorFromRequest(r, req.Actor),
		Reason:        req.Reason,
	})
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, policy)
}

func (s *Server) policyAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	writeJSON(w, s.store.ListPolicyAudits(q.Get("tenant_id"), q.Get("policy_id")))
}

func (s *Server) policyAssignments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		writeJSON(w, s.store.ListAssignments(q.Get("tenant_id"), q.Get("agent_id")))
	case http.MethodPost:
		if !s.requireOperator(w, r, "policy_admin") {
			return
		}
		var req policyAssignmentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode assignment: %v", err), http.StatusBadRequest)
			return
		}
		if req.Downlink && !s.requireOperator(w, r, "control_admin") {
			return
		}
		assignment := req.Assignment
		saved, ok := s.store.AssignPolicy(assignment)
		if !ok {
			http.Error(w, "policy not found or assignment invalid", http.StatusBadRequest)
			return
		}
		s.recordPolicyAudit(policymodel.AuditRecord{
			TenantID:      saved.TenantID,
			Action:        "policy.assign",
			PolicyID:      saved.PolicyID,
			PolicyVersion: saved.PolicyVersion,
			AssignmentID:  saved.AssignmentID,
			Actor:         s.actorFromRequest(r, req.Actor),
			Reason:        req.Reason,
		})
		var command *controlmodel.ControlCommand
		if req.Downlink {
			cmd, err := s.policyDownlinkCommand(r, saved, req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			out := s.store.CreateControlCommand(cmd)
			command = &out
		}
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		if command != nil {
			writeJSON(w, map[string]any{"assignment": saved, "control_command": command})
			return
		}
		writeJSON(w, saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) policyDownlinkCommand(r *http.Request, assignment policymodel.Assignment, req policyAssignmentRequest) (controlmodel.ControlCommand, error) {
	if strings.TrimSpace(assignment.AgentID) == "" {
		return controlmodel.ControlCommand{}, fmt.Errorf("downlink requires agent_id on policy assignment")
	}
	policy, ok := s.store.GetPolicy(assignment.TenantID, assignment.PolicyID, assignment.PolicyVersion)
	if !ok {
		return controlmodel.ControlCommand{}, fmt.Errorf("policy not found for downlink")
	}
	payload, err := json.Marshal(policy.EndpointPolicy())
	if err != nil {
		return controlmodel.ControlCommand{}, fmt.Errorf("encode policy downlink payload: %v", err)
	}
	return controlmodel.ControlCommand{
		CommandID:     req.CommandID,
		TenantID:      assignment.TenantID,
		AgentID:       assignment.AgentID,
		Type:          controlmodel.ControlCommandTypePolicyUpdate,
		PolicyID:      policy.PolicyID,
		PolicyVersion: policy.Version,
		PayloadJSON:   payload,
		Actor:         s.actorFromRequest(r, req.Actor),
		Reason:        firstNonEmptyString(req.Reason, "policy assignment downlink"),
	}, nil
}

func (s *Server) recordPolicyAudit(record policymodel.AuditRecord) {
	s.store.RecordPolicyAudit(record)
}

func (s *Server) effectivePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	policy, ok := s.store.EffectivePolicy(q.Get("tenant_id"), q.Get("agent_id"), q.Get("scope_type"), q.Get("scope_selector"))
	if !ok {
		http.Error(w, "effective policy not found", http.StatusNotFound)
		return
	}
	writeJSON(w, policy)
}
