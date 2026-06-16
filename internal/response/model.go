package response

import (
	"fmt"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

const (
	DefaultMode = "observe"
)

type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type Scope struct {
	Type     string `json:"type,omitempty"`
	Selector string `json:"selector,omitempty"`
}

type Intent struct {
	ResponseIntent    string `json:"response_intent,omitempty"`
	RecommendedAction string `json:"recommended_action,omitempty"`
	Confidence        uint32 `json:"confidence,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

type Command struct {
	ResponseID    string    `json:"response_id"`
	TenantID      string    `json:"tenant_id"`
	AgentID       string    `json:"agent_id"`
	PolicyID      string    `json:"policy_id,omitempty"`
	PolicyVersion uint64    `json:"policy_version,omitempty"`
	SignalID      string    `json:"signal_id,omitempty"`
	Scenario      string    `json:"scenario,omitempty"`
	Scope         Scope     `json:"scope,omitempty"`
	Action        string    `json:"action"`
	Mode          string    `json:"mode"`
	Target        string    `json:"target,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	Status        string    `json:"status"`
	Actor         string    `json:"actor,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

type Ack struct {
	ResponseID  string    `json:"response_id"`
	TenantID    string    `json:"tenant_id"`
	AgentID     string    `json:"agent_id"`
	Accepted    bool      `json:"accepted"`
	Unsupported bool      `json:"unsupported"`
	ObserveOnly bool      `json:"observe_only"`
	Executed    bool      `json:"executed"`
	Message     string    `json:"message,omitempty"`
	ObservedAt  time.Time `json:"observed_at,omitempty"`
}

type AuditRecord struct {
	Command Command `json:"command"`
	Ack     *Ack    `json:"ack,omitempty"`
}

func NormalizeCommand(cmd Command) Command {
	if cmd.TenantID == "" {
		cmd.TenantID = "default"
	}
	if cmd.Mode == "" {
		cmd.Mode = DefaultMode
	}
	if cmd.Action == "" {
		cmd.Action = "collect"
	}
	if cmd.Status == "" {
		cmd.Status = "pending"
	}
	now := time.Now().UTC()
	if cmd.CreatedAt.IsZero() {
		cmd.CreatedAt = now
	}
	cmd.UpdatedAt = now
	if cmd.ResponseID == "" {
		cmd.ResponseID = fmt.Sprintf("resp-%d", now.UnixNano())
	}
	return cmd
}

func ValidateCommand(cmd Command) Decision {
	mode := strings.TrimSpace(cmd.Mode)
	if mode == "" {
		mode = DefaultMode
	}
	if mode != "observe" {
		return Decision{Allowed: false, Reason: "only observe mode is allowed by default"}
	}
	switch strings.TrimSpace(cmd.Action) {
	case "", "collect", "noop":
		return Decision{Allowed: true}
	case "kill", "block", "quarantine":
		return Decision{Allowed: false, Reason: "destructive response action requires explicit policy approval"}
	default:
		return Decision{Allowed: false, Reason: "response action is not allowed by default"}
	}
}

func ScopeDecision(command, runtime Scope, runtimeKnown bool) Decision {
	command.Type = strings.TrimSpace(command.Type)
	command.Selector = strings.TrimSpace(command.Selector)
	runtime.Type = strings.TrimSpace(runtime.Type)
	runtime.Selector = strings.TrimSpace(runtime.Selector)
	if command.Type == "" && command.Selector == "" {
		return Decision{Allowed: true}
	}
	if !runtimeKnown || runtime.Type == "" {
		return Decision{Allowed: false, Reason: "agent runtime scope is required for scoped response command"}
	}
	if command.Type != runtime.Type || command.Selector != runtime.Selector {
		return Decision{Allowed: false, Reason: "response command scope does not match agent runtime scope"}
	}
	return Decision{Allowed: true}
}

func ToEnforcement(cmd Command) contract.EnforcementCmd {
	return contract.EnforcementCmd{
		ID:          cmd.ResponseID,
		Action:      cmd.Action,
		Target:      cmd.Target,
		ObserveOnly: cmd.Mode != "enforce",
		Reason:      cmd.Reason,
	}
}

func FromEnforcementAck(cmd Command, ack contract.EnforcementAck) Ack {
	return Ack{
		ResponseID:  cmd.ResponseID,
		TenantID:    cmd.TenantID,
		AgentID:     cmd.AgentID,
		Accepted:    ack.Accepted,
		Unsupported: ack.Unsupported,
		ObserveOnly: ack.ObserveOnly,
		Executed:    ack.Accepted && !ack.ObserveOnly && !ack.Unsupported,
		Message:     ack.Message,
		ObservedAt:  time.Now().UTC(),
	}
}
