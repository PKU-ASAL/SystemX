package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
)

type ControlResponseClient struct {
	Manager string
	Token   string
	Timeout time.Duration
	TLS     tlsconfig.ClientConfig
}

func NewControlResponseClient(manager, token string, timeout time.Duration) *ControlResponseClient {
	return NewControlResponseClientWithTLS(manager, token, timeout, tlsconfig.ClientConfig{})
}

func NewControlResponseClientWithTLS(manager, token string, timeout time.Duration, tlsCfg tlsconfig.ClientConfig) *ControlResponseClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ControlResponseClient{Manager: normalizeGRPCAddress(manager), Token: token, Timeout: timeout, TLS: tlsCfg}
}

func (c *ControlResponseClient) Pending(ctx context.Context, tenantID, agentID string) ([]responsemodel.Command, error) {
	frames, err := controlStreamHello(ctx, c.Manager, c.Token, c.Timeout, c.TLS, tenantID, agentID, "", "")
	if err != nil {
		return nil, err
	}
	var out []responsemodel.Command
	for _, frame := range frames {
		if frame.GetType() != "response_command" {
			continue
		}
		cmd, err := responseCommandFromControl(frame.GetResponseCommand())
		if err != nil {
			return nil, err
		}
		out = append(out, responsemodel.NormalizeCommand(cmd))
	}
	return out, nil
}

func (c *ControlResponseClient) Ack(ctx context.Context, ack responsemodel.Ack) error {
	if c == nil {
		return fmt.Errorf("control response client is nil")
	}
	return controlStreamSend(ctx, c.Manager, c.Token, c.Timeout, c.TLS, &controlv1.ControlStreamFrame{
		Type:      "response_ack",
		RequestId: ack.ResponseID,
		Context:   &controlv1.RequestContext{TenantId: ack.TenantID, AgentId: ack.AgentID},
		ResponseAck: &controlv1.ResponseAck{
			ResponseId:  ack.ResponseID,
			TenantId:    ack.TenantID,
			AgentId:     ack.AgentID,
			Accepted:    ack.Accepted,
			Unsupported: ack.Unsupported,
			ObserveOnly: ack.ObserveOnly,
			Executed:    ack.Executed,
			Message:     ack.Message,
			ObservedAt:  ack.ObservedAt.UTC().Format(time.RFC3339Nano),
		},
	})
}

func responseCommandFromControl(in *controlv1.ResponseCommand) (responsemodel.Command, error) {
	if in == nil {
		return responsemodel.Command{}, fmt.Errorf("control stream missing response_command")
	}
	if in.GetRawJson() != "" {
		var cmd responsemodel.Command
		if err := json.Unmarshal([]byte(in.GetRawJson()), &cmd); err != nil {
			return responsemodel.Command{}, fmt.Errorf("decode control stream response command raw_json: %w", err)
		}
		return cmd, nil
	}
	return responsemodel.Command{
		ResponseID:        in.GetResponseId(),
		TenantID:          in.GetTenantId(),
		AgentID:           in.GetAgentId(),
		PolicyID:          in.GetPolicyId(),
		PolicyVersion:     in.GetPolicyVersion(),
		SignalID:          in.GetSignalId(),
		Scenario:          in.GetScenario(),
		Scope:             responsemodel.Scope{Type: in.GetScope().GetType(), Selector: in.GetScope().GetSelector()},
		Action:            in.GetAction(),
		Mode:              in.GetMode(),
		Target:            in.GetTarget(),
		Reason:            in.GetReason(),
		Status:            in.GetStatus(),
		Actor:             in.GetActor(),
		ApprovalRequired:  in.GetApprovalRequired(),
		ApprovalStatus:    in.GetApprovalStatus(),
		ApprovalThreshold: in.GetApprovalThreshold(),
		ApprovalRoles:     append([]string(nil), in.GetApprovalRoles()...),
	}, nil
}
