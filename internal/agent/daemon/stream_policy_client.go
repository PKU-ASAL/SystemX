package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type StreamPolicyClient struct {
	Manager string
	Token   string
	Timeout time.Duration
}

type streamDownlinkEnvelope struct {
	Frames []streamDownlinkFrame `json:"frames"`
}

type streamDownlinkFrame struct {
	Type    string          `json:"type"`
	Version uint64          `json:"version"`
	Payload json.RawMessage `json:"payload"`
}

type streamPolicyPayload struct {
	TenantID      string   `json:"tenant_id"`
	PolicyID      string   `json:"policy_id"`
	PolicyVersion uint64   `json:"policy_version"`
	Mode          string   `json:"mode"`
	EndpointRules []string `json:"endpoint_rules"`
	CloudRules    []string `json:"cloud_rules"`
}

func NewStreamPolicyClient(manager, token string, timeout time.Duration) *StreamPolicyClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &StreamPolicyClient{Manager: normalizeGRPCAddress(manager), Token: token, Timeout: timeout}
}

func (c *StreamPolicyClient) EffectivePolicy(ctx context.Context, req EffectivePolicyRequest) (policymodel.Policy, error) {
	if c == nil {
		return policymodel.Policy{}, fmt.Errorf("stream policy client is nil")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	if c.Token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", c.Token)
	}
	conn, err := grpc.DialContext(ctx, c.Manager, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return policymodel.Policy{}, err
	}
	defer conn.Close()
	stream, err := analyticsv1.NewAgentGatewayClient(conn).Stream(ctx)
	if err != nil {
		return policymodel.Policy{}, err
	}
	hello, err := json.Marshal(map[string]string{
		"tenant_id":      req.TenantID,
		"agent_id":       req.AgentID,
		"scope_type":     req.ScopeType,
		"scope_selector": req.ScopeSelector,
	})
	if err != nil {
		return policymodel.Policy{}, err
	}
	if err := stream.Send(&analyticsv1.StreamFrame{Type: "hello", PayloadJson: hello}); err != nil {
		return policymodel.Policy{}, err
	}
	frame, err := stream.Recv()
	if err != nil {
		return policymodel.Policy{}, err
	}
	if frame.GetType() != "downlink" {
		return policymodel.Policy{}, fmt.Errorf("unexpected stream response type %q", frame.GetType())
	}
	var envelope streamDownlinkEnvelope
	if err := json.Unmarshal(frame.GetPayloadJson(), &envelope); err != nil {
		return policymodel.Policy{}, fmt.Errorf("decode stream downlink: %w", err)
	}
	for _, frame := range envelope.Frames {
		if frame.Type != "policy_update" {
			continue
		}
		var payload streamPolicyPayload
		if err := json.Unmarshal(frame.Payload, &payload); err != nil {
			return policymodel.Policy{}, fmt.Errorf("decode stream policy update: %w", err)
		}
		policy := policymodel.Policy{
			PolicyID:      payload.PolicyID,
			Version:       payload.PolicyVersion,
			TenantID:      payload.TenantID,
			EndpointRules: append([]string(nil), payload.EndpointRules...),
			CloudRules:    append([]string(nil), payload.CloudRules...),
			Mode:          payload.Mode,
			Published:     true,
		}
		if policy.PolicyID == "" {
			return policymodel.Policy{}, fmt.Errorf("stream policy missing policy_id")
		}
		return policymodel.Normalize(policy), nil
	}
	return policymodel.Policy{}, fmt.Errorf("stream downlink missing policy_update")
}

func normalizeGRPCAddress(manager string) string {
	manager = strings.TrimPrefix(manager, "http://")
	manager = strings.TrimPrefix(manager, "https://")
	return strings.TrimRight(manager, "/")
}
