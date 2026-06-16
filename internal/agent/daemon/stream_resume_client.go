package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type StreamResumeClient struct {
	Manager  string
	Token    string
	Timeout  time.Duration
	TenantID string
	AgentID  string
}

type streamResumePayload struct {
	TenantID     string `json:"tenant_id"`
	AgentID      string `json:"agent_id"`
	SessionID    string `json:"session_id,omitempty"`
	ResumeCursor string `json:"resume_cursor,omitempty"`
}

func NewStreamResumeClient(manager, token string, timeout time.Duration, tenantID, agentID string) *StreamResumeClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &StreamResumeClient{
		Manager:  normalizeGRPCAddress(manager),
		Token:    token,
		Timeout:  timeout,
		TenantID: tenantID,
		AgentID:  agentID,
	}
}

func (c *StreamResumeClient) ResumeCursor(ctx context.Context) (string, error) {
	if c == nil {
		return "", fmt.Errorf("stream resume client is nil")
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	if c.Token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", c.Token)
	}
	conn, err := grpc.DialContext(ctx, c.Manager, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return "", err
	}
	defer conn.Close()
	stream, err := analyticsv1.NewLink1Client(conn).Stream(ctx)
	if err != nil {
		return "", err
	}
	hello, err := json.Marshal(map[string]string{"tenant_id": c.TenantID, "agent_id": c.AgentID})
	if err != nil {
		return "", err
	}
	if err := stream.Send(&analyticsv1.StreamFrame{Type: "hello", PayloadJson: hello}); err != nil {
		return "", err
	}
	frame, err := stream.Recv()
	if err != nil {
		return "", err
	}
	if frame.GetType() != "downlink" {
		return "", fmt.Errorf("unexpected stream response type %q", frame.GetType())
	}
	var envelope streamDownlinkEnvelope
	if err := json.Unmarshal(frame.GetPayloadJson(), &envelope); err != nil {
		return "", fmt.Errorf("decode stream downlink: %w", err)
	}
	for _, frame := range envelope.Frames {
		if frame.Type != "resume" {
			continue
		}
		var payload streamResumePayload
		if err := json.Unmarshal(frame.Payload, &payload); err != nil {
			return "", fmt.Errorf("decode stream resume: %w", err)
		}
		if payload.AgentID != "" && payload.AgentID != c.AgentID {
			return "", fmt.Errorf("stream resume agent mismatch: got %q want %q", payload.AgentID, c.AgentID)
		}
		return payload.ResumeCursor, nil
	}
	return "", nil
}
