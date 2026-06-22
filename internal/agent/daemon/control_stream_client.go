package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func controlStreamHello(ctx context.Context, manager, token string, timeout time.Duration, tlsCfg tlsconfig.ClientConfig, tenantID, agentID, scopeType, scopeSelector string) ([]*controlv1.ControlStreamFrame, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", token)
	}
	creds, err := tlsconfig.ClientCredentials(tlsCfg)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.DialContext(ctx, manager, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stream, err := controlv1.NewAgentControlServiceClient(conn).ControlStream(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&controlv1.ControlStreamFrame{
		Type:      "hello",
		RequestId: "hello-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		Context: &controlv1.RequestContext{
			TenantId: tenantID,
			AgentId:  agentID,
			Scope:    &controlv1.Scope{Type: scopeType, Selector: scopeSelector},
		},
	}); err != nil {
		return nil, err
	}
	if err := stream.CloseSend(); err != nil {
		return nil, err
	}
	var frames []*controlv1.ControlStreamFrame
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return frames, nil
		}
		if err != nil {
			return nil, err
		}
		if frame.GetType() == "ack" && frame.GetAck().GetStatus() == "rejected" {
			return nil, fmt.Errorf("control stream hello rejected: %s", frame.GetAck().GetMessage())
		}
		frames = append(frames, frame)
	}
}

func controlStreamSend(ctx context.Context, manager, token string, timeout time.Duration, tlsCfg tlsconfig.ClientConfig, frame *controlv1.ControlStreamFrame) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", token)
	}
	creds, err := tlsconfig.ClientCredentials(tlsCfg)
	if err != nil {
		return err
	}
	conn, err := grpc.DialContext(ctx, manager, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer conn.Close()
	stream, err := controlv1.NewAgentControlServiceClient(conn).ControlStream(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(frame); err != nil {
		return err
	}
	reply, err := stream.Recv()
	if err != nil {
		return err
	}
	if reply.GetType() != "ack" {
		return fmt.Errorf("unexpected control stream response type %q", reply.GetType())
	}
	if reply.GetAck().GetStatus() != "accepted" {
		return fmt.Errorf("control stream frame rejected: %s", reply.GetAck().GetMessage())
	}
	return stream.CloseSend()
}
