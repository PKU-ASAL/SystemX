package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	gatewaymodel "github.com/sysarmor/sysarmor-next-project/internal/agentgateway/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type StreamEvidenceClient struct {
	Manager string
	Token   string
	Timeout time.Duration
}

func NewStreamEvidenceClient(manager, token string, timeout time.Duration) *StreamEvidenceClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &StreamEvidenceClient{Manager: normalizeGRPCAddress(manager), Token: token, Timeout: timeout}
}

func (c *StreamEvidenceClient) Pending(ctx context.Context, tenantID, agentID string) ([]gatewaymodel.EvidencePullbackRequest, error) {
	frame, err := c.downlink(ctx, tenantID, agentID)
	if err != nil {
		return nil, err
	}
	var envelope streamDownlinkEnvelope
	if err := json.Unmarshal(frame.GetPayloadJson(), &envelope); err != nil {
		return nil, fmt.Errorf("decode stream downlink: %w", err)
	}
	var out []gatewaymodel.EvidencePullbackRequest
	for _, frame := range envelope.Frames {
		if frame.Type != "evidence_pullback" {
			continue
		}
		var req gatewaymodel.EvidencePullbackRequest
		if err := json.Unmarshal(frame.Payload, &req); err != nil {
			return nil, fmt.Errorf("decode stream evidence pullback: %w", err)
		}
		out = append(out, gatewaymodel.NormalizeEvidencePullback(req))
	}
	return out, nil
}

func (c *StreamEvidenceClient) Result(ctx context.Context, result gatewaymodel.EvidencePullbackResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	if c.Token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", c.Token)
	}
	conn, err := grpc.DialContext(ctx, c.Manager, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer conn.Close()
	stream, err := analyticsv1.NewAgentGatewayClient(conn).Stream(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&analyticsv1.StreamFrame{Type: "evidence_pullback_result", PayloadJson: data}); err != nil {
		return err
	}
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	var out streamFrameResult
	if err := json.Unmarshal(frame.GetPayloadJson(), &out); err != nil {
		return fmt.Errorf("decode stream evidence pullback result: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("stream evidence pullback rejected: %s", out.Message)
	}
	return nil
}

func (c *StreamEvidenceClient) downlink(ctx context.Context, tenantID, agentID string) (*analyticsv1.StreamFrame, error) {
	if c == nil {
		return nil, fmt.Errorf("stream evidence client is nil")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	if c.Token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", c.Token)
	}
	conn, err := grpc.DialContext(ctx, c.Manager, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stream, err := analyticsv1.NewAgentGatewayClient(conn).Stream(ctx)
	if err != nil {
		return nil, err
	}
	hello, err := json.Marshal(map[string]string{"tenant_id": tenantID, "agent_id": agentID})
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&analyticsv1.StreamFrame{Type: "hello", PayloadJson: hello}); err != nil {
		return nil, err
	}
	frame, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	if frame.GetType() != "downlink" {
		return nil, fmt.Errorf("unexpected stream response type %q", frame.GetType())
	}
	return frame, nil
}
