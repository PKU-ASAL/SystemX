package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type StreamHealthReporter struct {
	Manager string
	Token   string
	Timeout time.Duration
}

func NewStreamHealthReporter(manager, token string, timeout time.Duration) *StreamHealthReporter {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &StreamHealthReporter{Manager: normalizeGRPCAddress(manager), Token: token, Timeout: timeout}
}

func (r *StreamHealthReporter) Report(ctx context.Context, health agenthealth.AgentHealth) error {
	if r == nil {
		return fmt.Errorf("stream health reporter is nil")
	}
	data, err := json.Marshal(health)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	if r.Token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", r.Token)
	}
	conn, err := grpc.DialContext(ctx, r.Manager, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer conn.Close()
	stream, err := analyticsv1.NewLink1Client(conn).Stream(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&analyticsv1.StreamFrame{Type: "health", PayloadJson: data}); err != nil {
		return err
	}
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	var result streamFrameResult
	if err := json.Unmarshal(frame.GetPayloadJson(), &result); err != nil {
		return fmt.Errorf("decode stream health result: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("stream health rejected: %s", result.Message)
	}
	return nil
}
