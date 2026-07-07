package managerapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
)

func (s *Server) responses(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		if q.Get("pending") == "true" {
			writeJSON(w, s.store.PendingResponses(q.Get("tenant_id"), q.Get("agent_id")))
			return
		}
		writeJSON(w, s.store.ListResponses(q.Get("tenant_id"), q.Get("agent_id")))
	case http.MethodPost:
		if !s.requireOperator(w, r, "responder") {
			return
		}
		var cmd responsemodel.Command
		if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
			http.Error(w, fmt.Sprintf("decode response command: %v", err), http.StatusBadRequest)
			return
		}
		cmd.Actor = s.actorFromRequest(r, cmd.Actor)
		s.createResponse(w, cmd)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) responseDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireOperator(w, r, "responder") {
		return
	}
	var req responseDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode response decision: %v", err), http.StatusBadRequest)
		return
	}
	if req.SignalID == "" {
		http.Error(w, "signal_id is required", http.StatusBadRequest)
		return
	}
	if req.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	sig, ok := s.store.GetSignal(req.SignalID)
	if !ok {
		http.Error(w, "signal not found", http.StatusNotFound)
		return
	}
	intent := sig.GetResponseIntent()
	if intent == nil || intent.GetResponseIntent() == "" {
		http.Error(w, "signal response intent not found", http.StatusBadRequest)
		return
	}
	action := intent.GetRecommendedAction()
	if action == "" {
		action = intent.GetResponseIntent()
	}
	target := req.Target
	if target == "" {
		target = signalResponseTarget(sig)
	}
	reason := fmt.Sprintf("signal=%s name=%s response_intent=%s confidence=%d", sig.GetId(), sig.GetName(), intent.GetResponseIntent(), intent.GetConfidence())
	if intent.GetReason() != "" {
		reason += " reason=" + intent.GetReason()
	}
	cmd := responsemodel.Command{
		ResponseID: "resp-" + sig.GetId(),
		TenantID:   req.TenantID,
		AgentID:    req.AgentID,
		SignalID:   sig.GetId(),
		Labels:     cloneStringMap(sig.GetLabels()),
		Scope:      req.Scope,
		Action:     action,
		Mode:       responsemodel.DefaultMode,
		Target:     target,
		Reason:     reason,
		Actor:      s.actorFromRequest(r, req.Actor),
	}
	s.createResponse(w, cmd)
}

func (s *Server) responseApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireOperator(w, r, "responder") {
		return
	}
	var req responseApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("decode response approval: %v", err), http.StatusBadRequest)
		return
	}
	if req.ResponseID == "" {
		http.Error(w, "response_id is required", http.StatusBadRequest)
		return
	}
	cmd, ok := s.store.ApproveResponse(req.TenantID, req.AgentID, req.ResponseID, req.Approved, s.actorFromRequest(r, req.Actor), s.roleFromRequest(r, req.Role), req.Reason)
	if !ok {
		http.Error(w, "response command not found", http.StatusNotFound)
		return
	}
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, responsemodel.AuditRecord{Command: cmd})
}

func (s *Server) createResponse(w http.ResponseWriter, cmd responsemodel.Command) {
	if cmd.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	if cmd.TenantID == "" {
		cmd.TenantID = "default"
	}
	if health, ok := s.store.GetAgentHealth(cmd.TenantID, cmd.AgentID); ok {
		if cmd.Scope.Type == "" && cmd.Scope.Selector == "" {
			cmd.Scope = responsemodel.Scope{Type: health.Scope.Type, Selector: health.Scope.Selector}
		}
		if decision := responsemodel.ScopeDecision(cmd.Scope, responsemodel.Scope{Type: health.Scope.Type, Selector: health.Scope.Selector}, true); !decision.Allowed {
			s.denyResponse(w, cmd, decision)
			return
		}
	} else if cmd.Scope.Type != "" || cmd.Scope.Selector != "" {
		s.denyResponse(w, cmd, responsemodel.Decision{Allowed: false, Reason: "agent runtime scope is required for scoped response command"})
		return
	}
	responsePolicy := responsemodel.DefaultPolicy()
	if policy, ok := s.store.EffectivePolicy(cmd.TenantID, cmd.AgentID, cmd.Scope.Type, cmd.Scope.Selector); ok {
		responsePolicy = policy.Response
		if len(responsePolicy.AllowedActions) == 0 && len(responsePolicy.AllowedModes) == 0 {
			responsePolicy = responsemodel.DefaultPolicy()
		}
		if cmd.PolicyID == "" {
			cmd.PolicyID = policy.PolicyID
			cmd.PolicyVersion = policy.Version
		}
	}
	if cmd.PolicyID == "" {
		policy, _ := s.store.EffectivePolicy(cmd.TenantID, cmd.AgentID, cmd.Scope.Type, cmd.Scope.Selector)
		cmd.PolicyID = policy.PolicyID
		cmd.PolicyVersion = policy.Version
	}
	cmd = responsemodel.ApplyPolicyRequirements(cmd, responsePolicy)
	if decision := responsemodel.ValidateCommandWithPolicy(cmd, responsePolicy); !decision.Allowed {
		s.denyResponse(w, cmd, decision)
		return
	}
	if cmd.ApprovalRequired {
		cmd = responsemodel.NormalizeCommand(cmd)
		cmd.Status = "pending_approval"
		cmd.ApprovalStatus = "required"
	}
	cmd = s.store.CreateResponse(cmd)
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cmd)
}

func (s *Server) denyResponse(w http.ResponseWriter, cmd responsemodel.Command, decision responsemodel.Decision) {
	cmd = responsemodel.NormalizeCommand(cmd)
	cmd.Status = "denied"
	if cmd.Reason == "" {
		cmd.Reason = decision.Reason
	} else {
		cmd.Reason = cmd.Reason + "; denied: " + decision.Reason
	}
	cmd = s.store.CreateResponse(cmd)
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusForbidden)
	writeJSON(w, responsemodel.AuditRecord{Command: cmd})
}

func signalResponseTarget(sig *signalv1.Signal) string {
	for _, entity := range sig.GetEntities() {
		if entity.GetKind() == "process" && entity.GetKey() != "" {
			return entity.GetKey()
		}
	}
	for _, entity := range sig.GetEntities() {
		if entity.GetKey() != "" {
			return entity.GetKey()
		}
	}
	return ""
}

func (s *Server) responseAcks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireOperator(w, r, "response_admin") {
		return
	}
	var ack responsemodel.Ack
	if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
		http.Error(w, fmt.Sprintf("decode response ack: %v", err), http.StatusBadRequest)
		return
	}
	if ack.AgentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}
	if _, ok := s.store.AckResponse(ack); !ok {
		http.Error(w, "response command not found", http.StatusNotFound)
		return
	}
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}
