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

type Policy struct {
	AllowedActions    []string `json:"allowed_actions,omitempty"`
	AllowedModes      []string `json:"allowed_modes,omitempty"`
	ApprovalRequired  bool     `json:"approval_required,omitempty"`
	ApprovalThreshold uint32   `json:"approval_threshold,omitempty"`
	ApprovalRoles     []string `json:"approval_roles,omitempty"`
	AllowDestructive  bool     `json:"allow_destructive,omitempty"`
}

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
	ResponseID        string     `json:"response_id"`
	TenantID          string     `json:"tenant_id"`
	AgentID           string     `json:"agent_id"`
	PolicyID          string     `json:"policy_id,omitempty"`
	PolicyVersion     uint64     `json:"policy_version,omitempty"`
	SignalID          string     `json:"signal_id,omitempty"`
	Scenario          string     `json:"scenario,omitempty"`
	Scope             Scope      `json:"scope,omitempty"`
	Action            string     `json:"action"`
	Mode              string     `json:"mode"`
	Target            string     `json:"target,omitempty"`
	Reason            string     `json:"reason,omitempty"`
	Status            string     `json:"status"`
	Actor             string     `json:"actor,omitempty"`
	ApprovalRequired  bool       `json:"approval_required,omitempty"`
	ApprovalStatus    string     `json:"approval_status,omitempty"`
	ApprovalThreshold uint32     `json:"approval_threshold,omitempty"`
	ApprovalRoles     []string   `json:"approval_roles,omitempty"`
	Approvals         []Approval `json:"approvals,omitempty"`
	ApprovedBy        string     `json:"approved_by,omitempty"`
	ApprovedAt        time.Time  `json:"approved_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at,omitempty"`
}

type Approval struct {
	Actor      string    `json:"actor,omitempty"`
	Role       string    `json:"role,omitempty"`
	Approved   bool      `json:"approved"`
	Reason     string    `json:"reason,omitempty"`
	ObservedAt time.Time `json:"observed_at,omitempty"`
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
	return ValidateCommandWithPolicy(cmd, DefaultPolicy())
}

func DefaultPolicy() Policy {
	return Policy{
		AllowedActions: []string{"collect", "noop"},
		AllowedModes:   []string{DefaultMode},
	}
}

func ValidateCommandWithPolicy(cmd Command, policy Policy) Decision {
	mode := strings.TrimSpace(cmd.Mode)
	if mode == "" {
		mode = DefaultMode
	}
	if len(policy.AllowedModes) == 0 {
		policy.AllowedModes = []string{DefaultMode}
	}
	if !containsTrimmed(policy.AllowedModes, mode) {
		return Decision{Allowed: false, Reason: "response mode is not allowed by policy"}
	}
	action := strings.TrimSpace(cmd.Action)
	if action == "" {
		action = "collect"
	}
	if isDestructiveAction(action) && !policy.AllowDestructive {
		return Decision{Allowed: false, Reason: "destructive response action requires explicit policy approval"}
	}
	if len(policy.AllowedActions) == 0 {
		policy.AllowedActions = []string{"collect", "noop"}
	}
	if !containsTrimmed(policy.AllowedActions, action) {
		return Decision{Allowed: false, Reason: "response action is not allowed by policy"}
	}
	return Decision{Allowed: true}
}

func ApplyPolicyRequirements(cmd Command, policy Policy) Command {
	if policy.ApprovalRequired {
		cmd.ApprovalRequired = true
		if policy.ApprovalThreshold > 0 {
			cmd.ApprovalThreshold = policy.ApprovalThreshold
		}
		if len(policy.ApprovalRoles) > 0 {
			cmd.ApprovalRoles = append([]string(nil), policy.ApprovalRoles...)
		}
	}
	return cmd
}

func ApprovalThreshold(cmd Command) uint32 {
	if cmd.ApprovalThreshold == 0 {
		return 1
	}
	return cmd.ApprovalThreshold
}

func ApprovalRoleAllowed(cmd Command, role string) bool {
	if len(cmd.ApprovalRoles) == 0 {
		return true
	}
	role = strings.TrimSpace(role)
	if role == "admin" {
		return true
	}
	return containsTrimmed(cmd.ApprovalRoles, role)
}

func ApprovalCount(cmd Command) uint32 {
	seen := map[string]bool{}
	var count uint32
	for _, approval := range cmd.Approvals {
		if !approval.Approved {
			continue
		}
		if !ApprovalRoleAllowed(cmd, approval.Role) {
			continue
		}
		key := strings.TrimSpace(approval.Actor)
		if key == "" {
			key = strings.TrimSpace(approval.Role)
		}
		if key == "" {
			key = fmt.Sprintf("approval-%d", count+1)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
	}
	return count
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

func containsTrimmed(values []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func isDestructiveAction(action string) bool {
	switch strings.TrimSpace(action) {
	case "kill", "block", "quarantine":
		return true
	default:
		return false
	}
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
