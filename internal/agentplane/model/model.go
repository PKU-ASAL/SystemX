package controlmodel

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	EvidencePullbackStatusPending   = "pending"
	EvidencePullbackStatusCompleted = "completed"
	EvidencePullbackStatusFailed    = "failed"
)

type EvidencePullbackRequest struct {
	RequestID   string    `json:"request_id"`
	TenantID    string    `json:"tenant_id"`
	AgentID     string    `json:"agent_id"`
	IncidentID  string    `json:"incident_id,omitempty"`
	Scenario    string    `json:"scenario,omitempty"`
	Target      string    `json:"target,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Status      string    `json:"status"`
	ResultOK    bool      `json:"result_ok,omitempty"`
	Result      string    `json:"result,omitempty"`
	Actor       string    `json:"actor,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

type EvidencePullbackResult struct {
	RequestID  string          `json:"request_id"`
	TenantID   string          `json:"tenant_id"`
	AgentID    string          `json:"agent_id"`
	OK         bool            `json:"ok"`
	Message    string          `json:"message,omitempty"`
	Evidence   json.RawMessage `json:"evidence,omitempty"`
	ObservedAt time.Time       `json:"observed_at,omitempty"`
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
