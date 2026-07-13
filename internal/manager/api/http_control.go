package managerapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
)

func (s *Server) evidencePullbacks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		writeJSON(w, s.store.ListEvidencePullbacks(q.Get("tenant_id"), q.Get("agent_id")))
	case http.MethodPost:
		if !s.requireOperator(w, r, "incident_admin") {
			return
		}
		var req evidencePullbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode evidence pullback: %v", err), http.StatusBadRequest)
			return
		}
		if req.AgentID == "" {
			http.Error(w, "agent_id is required", http.StatusBadRequest)
			return
		}
		out := s.store.CreateEvidencePullback(controlmodel.EvidencePullbackRequest{
			RequestID:  req.RequestID,
			TenantID:   req.TenantID,
			AgentID:    req.AgentID,
			IncidentID: req.IncidentID,
			Labels:     cloneStringMap(req.Labels),
			Target:     req.Target,
			Reason:     req.Reason,
			Actor:      s.actorFromRequest(r, req.Actor),
		})
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, out)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) controlCommands(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		writeJSON(w, s.store.ListControlCommands(q.Get("tenant_id"), q.Get("agent_id"), q.Get("type")))
	case http.MethodPost:
		if !s.requireOperator(w, r, "control_admin") {
			return
		}
		var req controlCommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode control command: %v", err), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Action) != "" {
			s.controlCommandAction(w, r, req)
			return
		}
		cmd, err := s.controlCommandFromRequest(r, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		out := s.store.CreateControlCommand(cmd)
		if err := s.store.Save(); err != nil {
			http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
			return
		}
		writeJSON(w, out)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) controlCommandAction(w http.ResponseWriter, r *http.Request, req controlCommandRequest) {
	commandID := strings.TrimSpace(req.CommandID)
	if commandID == "" {
		http.Error(w, "command_id is required", http.StatusBadRequest)
		return
	}
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	actor := s.actorFromRequest(r, req.Actor)
	var (
		out controlmodel.ControlCommand
		ok  bool
	)
	switch strings.TrimSpace(req.Action) {
	case "cancel":
		out, ok = s.store.CancelControlCommand(commandID, tenantID, req.AgentID, actor, req.Reason)
	case "retry":
		out, ok = s.store.RetryControlCommand(commandID, tenantID, req.AgentID, actor, req.Reason)
	case "expire":
		out, ok = s.store.ExpireControlCommand(commandID, tenantID, req.AgentID, req.Reason)
	default:
		http.Error(w, fmt.Sprintf("unsupported control command action %q", req.Action), http.StatusBadRequest)
		return
	}
	if !ok {
		http.Error(w, "control command not found", http.StatusNotFound)
		return
	}
	if err := s.store.Save(); err != nil {
		http.Error(w, fmt.Sprintf("save store: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, out)
}

func (s *Server) controlCommandFromRequest(r *http.Request, req controlCommandRequest) (controlmodel.ControlCommand, error) {
	commandType := strings.TrimSpace(req.Type)
	if commandType == "" {
		return controlmodel.ControlCommand{}, fmt.Errorf("type is required")
	}
	if strings.TrimSpace(req.AgentID) == "" {
		return controlmodel.ControlCommand{}, fmt.Errorf("agent_id is required")
	}
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	payload := append(json.RawMessage(nil), req.PayloadJSON...)
	policyID := req.PolicyID
	policyVersion := req.PolicyVersion
	switch commandType {
	case controlmodel.ControlCommandTypePolicyUpdate:
		if len(payload) == 0 {
			if policyID == "" {
				return controlmodel.ControlCommand{}, fmt.Errorf("policy_id or payload_json is required for policy_update")
			}
			policy, ok := s.store.GetPolicy(tenantID, policyID, policyVersion)
			if !ok {
				return controlmodel.ControlCommand{}, fmt.Errorf("policy not found")
			}
			raw, err := json.Marshal(policy.EndpointPolicy())
			if err != nil {
				return controlmodel.ControlCommand{}, fmt.Errorf("encode policy payload: %v", err)
			}
			payload = raw
			policyID = policy.PolicyID
			policyVersion = policy.Version
		} else if policyID == "" || policyVersion == 0 {
			if parsedID, parsedVersion := policyMetadataFromPayload(payload); policyID == "" || policyVersion == 0 {
				if policyID == "" {
					policyID = parsedID
				}
				if policyVersion == 0 {
					policyVersion = parsedVersion
				}
			}
		}
	case controlmodel.ControlCommandTypeContentUpdate:
		if len(payload) == 0 {
			return controlmodel.ControlCommand{}, fmt.Errorf("payload_json is required for content_update")
		}
		if req.ContentRef == "" || req.ContentKind == "" || req.ContentVersion == "" {
			ref, kind, version := contentMetadataFromPayload(payload)
			if req.ContentRef == "" {
				req.ContentRef = ref
			}
			if req.ContentKind == "" {
				req.ContentKind = kind
			}
			if req.ContentVersion == "" {
				req.ContentVersion = version
			}
		}
	default:
		return controlmodel.ControlCommand{}, fmt.Errorf("unsupported control command type %q", commandType)
	}
	return controlmodel.ControlCommand{
		CommandID:      req.CommandID,
		TenantID:       tenantID,
		AgentID:        req.AgentID,
		Type:           commandType,
		PolicyID:       policyID,
		PolicyVersion:  policyVersion,
		ContentRef:     req.ContentRef,
		ContentKind:    req.ContentKind,
		ContentVersion: req.ContentVersion,
		PayloadJSON:    payload,
		Actor:          s.actorFromRequest(r, req.Actor),
		Reason:         req.Reason,
	}, nil
}

func policyMetadataFromPayload(payload json.RawMessage) (string, uint64) {
	var policy struct {
		PolicyID string `json:"policy_id"`
		Version  uint64 `json:"version"`
	}
	_ = json.Unmarshal(payload, &policy)
	return policy.PolicyID, policy.Version
}

func contentMetadataFromPayload(payload json.RawMessage) (string, string, string) {
	var content struct {
		Kind     string `json:"kind"`
		Metadata struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(payload, &content)
	return content.Metadata.ID, content.Kind, content.Metadata.Version
}
