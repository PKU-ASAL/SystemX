package controlmodel

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	UnenrollmentProtocolLegacyMTLS   = "legacy_mtls"
	UnenrollmentProtocolCompletionV1 = "completion_v1"

	EvidencePullbackStatusPending   = "pending"
	EvidencePullbackStatusCompleted = "completed"
	EvidencePullbackStatusFailed    = "failed"

	ControlCommandTypePolicyUpdate  = "policy_update"
	ControlCommandTypeContentUpdate = "content_update"

	ControlCommandStatusPending  = "pending"
	ControlCommandStatusSent     = "sent"
	ControlCommandStatusApplied  = "applied"
	ControlCommandStatusRejected = "rejected"
	ControlCommandStatusFailed   = "failed"
	ControlCommandStatusCanceled = "canceled"
	ControlCommandStatusExpired  = "expired"
)

type ControlCommand struct {
	CommandID      string          `json:"command_id"`
	TenantID       string          `json:"tenant_id"`
	AgentID        string          `json:"agent_id"`
	Type           string          `json:"type"`
	Status         string          `json:"status"`
	PolicyID       string          `json:"policy_id,omitempty"`
	PolicyVersion  uint64          `json:"policy_version,omitempty"`
	ContentRef     string          `json:"content_ref,omitempty"`
	ContentKind    string          `json:"content_kind,omitempty"`
	ContentVersion string          `json:"content_version,omitempty"`
	PayloadJSON    json.RawMessage `json:"payload_json,omitempty"`
	Actor          string          `json:"actor,omitempty"`
	Reason         string          `json:"reason,omitempty"`
	CreatedAt      time.Time       `json:"created_at,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at,omitempty"`
	SentAt         time.Time       `json:"sent_at,omitempty"`
	LastSentAt     time.Time       `json:"last_sent_at,omitempty"`
	AckedAt        time.Time       `json:"acked_at,omitempty"`
	CanceledAt     time.Time       `json:"canceled_at,omitempty"`
	ExpiredAt      time.Time       `json:"expired_at,omitempty"`
	AttemptCount   uint32          `json:"attempt_count,omitempty"`
	AckStatus      string          `json:"ack_status,omitempty"`
	AckMessage     string          `json:"ack_message,omitempty"`
	AckPolicyID    string          `json:"ack_policy_id,omitempty"`
	AckPolicyVer   uint64          `json:"ack_policy_version,omitempty"`
	AckReportJSON  string          `json:"ack_report_json,omitempty"`
	Error          string          `json:"error,omitempty"`
}

type ControlCommandAck struct {
	CommandID     string    `json:"command_id"`
	TenantID      string    `json:"tenant_id"`
	AgentID       string    `json:"agent_id"`
	Status        string    `json:"status"`
	Message       string    `json:"message,omitempty"`
	PolicyID      string    `json:"policy_id,omitempty"`
	PolicyVersion uint64    `json:"policy_version,omitempty"`
	ReportJSON    string    `json:"report_json,omitempty"`
	ObservedAt    time.Time `json:"observed_at,omitempty"`
}

type EvidencePullbackRequest struct {
	RequestID   string            `json:"request_id"`
	TenantID    string            `json:"tenant_id"`
	AgentID     string            `json:"agent_id"`
	IncidentID  string            `json:"incident_id,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Target      string            `json:"target,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	Status      string            `json:"status"`
	ResultOK    bool              `json:"result_ok,omitempty"`
	Result      string            `json:"result,omitempty"`
	Actor       string            `json:"actor,omitempty"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
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

func NormalizeControlCommand(cmd ControlCommand) ControlCommand {
	cmd.Type = strings.TrimSpace(cmd.Type)
	if cmd.TenantID == "" {
		cmd.TenantID = "default"
	}
	if cmd.Status == "" {
		cmd.Status = ControlCommandStatusPending
	}
	now := time.Now().UTC()
	if cmd.CreatedAt.IsZero() {
		cmd.CreatedAt = now
	}
	cmd.UpdatedAt = now
	if cmd.CommandID == "" {
		cmd.CommandID = fmt.Sprintf("ctrl-%d", now.UnixNano())
	}
	return cmd
}

func ControlCommandTerminalStatus(status string) bool {
	switch status {
	case ControlCommandStatusApplied, ControlCommandStatusRejected, ControlCommandStatusFailed, ControlCommandStatusCanceled, ControlCommandStatusExpired:
		return true
	default:
		return false
	}
}
