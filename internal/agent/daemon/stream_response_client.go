package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type StreamResponseClient struct {
	Manager string
	Token   string
	Timeout time.Duration
}

func NewStreamResponseClient(manager, token string, timeout time.Duration) *StreamResponseClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &StreamResponseClient{Manager: normalizeGRPCAddress(manager), Token: token, Timeout: timeout}
}

func (c *StreamResponseClient) Pending(ctx context.Context, tenantID, agentID string) ([]responsemodel.Command, error) {
	frame, err := c.downlink(ctx, tenantID, agentID)
	if err != nil {
		return nil, err
	}
	var envelope streamDownlinkEnvelope
	if err := json.Unmarshal(frame.GetPayloadJson(), &envelope); err != nil {
		return nil, fmt.Errorf("decode stream downlink: %w", err)
	}
	var out []responsemodel.Command
	for _, frame := range envelope.Frames {
		if frame.Type != "response_command" {
			continue
		}
		var cmd responsemodel.Command
		if err := json.Unmarshal(frame.Payload, &cmd); err != nil {
			return nil, fmt.Errorf("decode stream response command: %w", err)
		}
		out = append(out, responsemodel.NormalizeCommand(cmd))
	}
	return out, nil
}

func (c *StreamResponseClient) Ack(ctx context.Context, ack responsemodel.Ack) error {
	data, err := json.Marshal(ack)
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
	stream, err := analyticsv1.NewLink1Client(conn).Stream(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&analyticsv1.StreamFrame{Type: "ack", PayloadJson: data}); err != nil {
		return err
	}
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	var result streamFrameResult
	if err := json.Unmarshal(frame.GetPayloadJson(), &result); err != nil {
		return fmt.Errorf("decode stream ack result: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("stream ack rejected: %s", result.Message)
	}
	return nil
}

func (c *StreamResponseClient) downlink(ctx context.Context, tenantID, agentID string) (*analyticsv1.StreamFrame, error) {
	if c == nil {
		return nil, fmt.Errorf("stream response client is nil")
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
	stream, err := analyticsv1.NewLink1Client(conn).Stream(ctx)
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

type streamFrameResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}
