package daemon

import (
	"context"
	"fmt"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type ControlStreamHealthReporter struct {
	Manager string
	Token   string
	Timeout time.Duration
	TLS     tlsconfig.ClientConfig
}

func NewControlStreamHealthReporter(manager, token string, timeout time.Duration) *ControlStreamHealthReporter {
	return NewControlStreamHealthReporterWithTLS(manager, token, timeout, tlsconfig.ClientConfig{})
}

func NewControlStreamHealthReporterWithTLS(manager, token string, timeout time.Duration, tlsCfg tlsconfig.ClientConfig) *ControlStreamHealthReporter {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &ControlStreamHealthReporter{Manager: normalizeGRPCAddress(manager), Token: token, Timeout: timeout, TLS: tlsCfg}
}

func (r *ControlStreamHealthReporter) Report(ctx context.Context, health agenthealth.AgentHealth) error {
	if r == nil {
		return fmt.Errorf("control stream health reporter is nil")
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	if r.Token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", r.Token)
	}
	creds, err := tlsconfig.ClientCredentials(r.TLS)
	if err != nil {
		return err
	}
	conn, err := grpc.DialContext(ctx, r.Manager, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer conn.Close()
	stream, err := controlv1.NewAgentControlServiceClient(conn).ControlStream(ctx)
	if err != nil {
		return err
	}
	frame := &controlv1.ControlStreamFrame{
		Type:            "health_report",
		RequestId:       "health-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		ContractVersion: 1,
		Sequence:        1,
		Context: &controlv1.RequestContext{
			TenantId: health.TenantID,
			AgentId:  health.AgentID,
			Scope:    &controlv1.Scope{Type: health.Scope.Type, Selector: health.Scope.Selector},
		},
		Health: healthResponse(health),
	}
	if err := stream.Send(frame); err != nil {
		return err
	}
	ack, err := stream.Recv()
	if err != nil {
		return err
	}
	if ack.GetAck().GetStatus() != "accepted" {
		return fmt.Errorf("control stream health rejected: %s", ack.GetAck().GetMessage())
	}
	return stream.CloseSend()
}
