package link1

import (
	"fmt"
	"time"
)

const EvidencePullbackStatusPending = "pending"

type EvidencePullbackRequest struct {
	RequestID  string    `json:"request_id"`
	TenantID   string    `json:"tenant_id"`
	AgentID    string    `json:"agent_id"`
	IncidentID string    `json:"incident_id,omitempty"`
	Scenario   string    `json:"scenario,omitempty"`
	Target     string    `json:"target,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	Status     string    `json:"status"`
	Actor      string    `json:"actor,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

func NormalizeEvidencePullback(req EvidencePullbackRequest) EvidencePullbackRequest {
	if req.TenantID == "" {
		req.TenantID = "default"
	}
	if req.Status == "" {
		req.Status = EvidencePullbackStatusPending
	}
	now := time.Now().UTC()
	if req.CreatedAt.IsZero() {
		req.CreatedAt = now
	}
	req.UpdatedAt = now
	if req.RequestID == "" {
		req.RequestID = fmt.Sprintf("evpb-%d", now.UnixNano())
	}
	return req
}
