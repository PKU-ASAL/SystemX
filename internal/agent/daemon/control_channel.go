package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type ControlChannel struct {
	manager string
	token   string
	tls     tlsconfig.ClientConfig

	conn   *grpc.ClientConn
	stream controlplanev1.AgentControlPlaneService_ConnectClient
	next   uint64
}

func NewControlChannel(manager, token string, tlsCfg tlsconfig.ClientConfig) *ControlChannel {
	return &ControlChannel{manager: normalizeGRPCAddress(manager), token: token, tls: tlsCfg, next: 1}
}

func (s *ControlChannel) Open(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("control channel session is nil")
	}
	if s.stream != nil {
		return nil
	}
	if s.token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", s.token)
	}
	creds, err := tlsconfig.ClientCredentials(s.tls)
	if err != nil {
		return err
	}
	conn, err := grpc.DialContext(ctx, s.manager, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		return err
	}
	stream, err := controlplanev1.NewAgentControlPlaneServiceClient(conn).Connect(context.WithoutCancel(ctx))
	if err != nil {
		_ = conn.Close()
		return err
	}
	s.conn = conn
	s.stream = stream
	return nil
}

func (s *ControlChannel) Hello(ctx context.Context, tenantID, agentID, scopeType, scopeSelector string) ([]*controlplanev1.ControlFrame, error) {
	requestID := "hello-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if err := s.Send(ctx, &controlplanev1.ControlFrame{
		Type:      "hello",
		RequestId: requestID,
		Context: &controlplanev1.RequestContext{
			TenantId: tenantID,
			AgentId:  agentID,
			Scope:    &controlplanev1.Scope{Type: scopeType, Selector: scopeSelector},
		},
	}); err != nil {
		return nil, err
	}
	var frames []*controlplanev1.ControlFrame
	for {
		frame, err := s.Recv()
		if err != nil {
			return nil, err
		}
		if frame.GetRequestId() != requestID {
			frames = append(frames, frame)
			continue
		}
		if frame.GetType() == "ack" && frame.GetAck().GetStatus() == "rejected" {
			return nil, fmt.Errorf("control channel hello rejected: %s", frame.GetAck().GetMessage())
		}
		frames = append(frames, frame)
		if frame.GetType() == "resume" {
			return frames, nil
		}
	}
}

func (s *ControlChannel) ReportHealth(ctx context.Context, health agenthealth.AgentHealth) error {
	if err := s.SendHealth(ctx, health); err != nil {
		return err
	}
	reply, err := s.Recv()
	if err != nil {
		return err
	}
	if reply.GetType() != "ack" || reply.GetAck().GetStatus() != "accepted" {
		return fmt.Errorf("control channel health rejected: %s", reply.GetAck().GetMessage())
	}
	return nil
}

func (s *ControlChannel) SendHealth(ctx context.Context, health agenthealth.AgentHealth) error {
	return s.Send(ctx, &controlplanev1.ControlFrame{
		Type:      "health_report",
		RequestId: "health-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		Context: &controlplanev1.RequestContext{
			TenantId: health.TenantID,
			AgentId:  health.AgentID,
			Scope:    &controlplanev1.Scope{Type: health.Scope.Type, Selector: health.Scope.Selector},
		},
		Health: healthResponse(health),
	})
}

func (s *ControlChannel) SendResponseAck(ctx context.Context, ack responsemodel.Ack) error {
	return s.Send(ctx, &controlplanev1.ControlFrame{
		Type:      "response_ack",
		RequestId: ack.ResponseID,
		Context:   &controlplanev1.RequestContext{TenantId: ack.TenantID, AgentId: ack.AgentID},
		ResponseAck: &controlplanev1.ResponseAck{
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

func (s *ControlChannel) SendEvidenceResult(ctx context.Context, result controlmodel.EvidencePullbackResult) error {
	return s.Send(ctx, &controlplanev1.ControlFrame{
		Type:      "evidence_pullback_result",
		RequestId: result.RequestID,
		Context:   &controlplanev1.RequestContext{TenantId: result.TenantID, AgentId: result.AgentID},
		EvidenceResult: &controlplanev1.EvidencePullbackResult{
			RequestId:    result.RequestID,
			TenantId:     result.TenantID,
			AgentId:      result.AgentID,
			Ok:           result.OK,
			Message:      result.Message,
			EvidenceJson: append([]byte(nil), result.Evidence...),
			ObservedAt:   result.ObservedAt.UTC().Format(time.RFC3339Nano),
		},
	})
}

func (s *ControlChannel) SendControlAck(ctx context.Context, ack *controlplanev1.ControlAck) error {
	if ack == nil {
		return fmt.Errorf("control ack is nil")
	}
	requestID := ack.GetRequestId()
	if requestID == "" {
		requestID = "ack-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	}
	return s.Send(ctx, &controlplanev1.ControlFrame{
		Type:      "ack",
		RequestId: requestID,
		Context: &controlplanev1.RequestContext{
			RequestId: requestID,
			TenantId:  ack.GetTenantId(),
			AgentId:   ack.GetAgentId(),
		},
		Ack: ack,
	})
}

func (s *ControlChannel) SendCapability(ctx context.Context, health agenthealth.AgentHealth) error {
	return s.Send(ctx, &controlplanev1.ControlFrame{
		Type:      "capability_report",
		RequestId: "capability-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		Context: &controlplanev1.RequestContext{
			TenantId: health.TenantID,
			AgentId:  health.AgentID,
			Scope:    &controlplanev1.Scope{Type: health.Scope.Type, Selector: health.Scope.Selector},
		},
		Capability: capabilityResponse(health),
	})
}

func capabilityResponse(health agenthealth.AgentHealth) *controlplanev1.CapabilityResponse {
	return &controlplanev1.CapabilityResponse{
		AgentId:                  health.AgentID,
		HostId:                   health.HostID,
		TenantId:                 health.TenantID,
		Scope:                    scopeMessage(health.Scope),
		Sensor:                   capabilityMessage(health.Capability),
		SupportedPolicySections:  []string{"collection", "detection", "data_plane"},
		SupportedResponseActions: []string{"collect", "noop"},
		CollectionBehaviors:      collectionBehaviorMessages(health.Capability.Collection),
	}
}

func (s *ControlChannel) Send(ctx context.Context, frame *controlplanev1.ControlFrame) error {
	if s == nil {
		return fmt.Errorf("control channel session is nil")
	}
	if frame == nil {
		return fmt.Errorf("control channel frame is nil")
	}
	if frame.GetRequestId() == "" {
		return fmt.Errorf("control channel request_id is required")
	}
	if s.stream == nil {
		if err := s.Open(ctx); err != nil {
			return err
		}
	}
	frame.ContractVersion = 1
	frame.Sequence = s.next
	s.next++
	return s.stream.Send(frame)
}

func (s *ControlChannel) Recv() (*controlplanev1.ControlFrame, error) {
	if s == nil || s.stream == nil {
		return nil, fmt.Errorf("control channel session is not open")
	}
	frame, err := s.stream.Recv()
	if errors.Is(err, io.EOF) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return frame, nil
}

func (s *ControlChannel) Close() error {
	if s == nil {
		return nil
	}
	var err error
	if s.stream != nil {
		err = s.stream.CloseSend()
		s.stream = nil
	}
	if s.conn != nil {
		if closeErr := s.conn.Close(); err == nil {
			err = closeErr
		}
		s.conn = nil
	}
	return err
}
