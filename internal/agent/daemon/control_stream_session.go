package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	gatewaymodel "github.com/sysarmor/sysarmor-next-project/internal/agentplane/model"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type ControlStreamSession struct {
	manager string
	token   string
	tls     tlsconfig.ClientConfig

	conn   *grpc.ClientConn
	stream controlv1.AgentControlService_ControlStreamClient
	next   uint64
}

func NewControlStreamSession(manager, token string, tlsCfg tlsconfig.ClientConfig) *ControlStreamSession {
	return &ControlStreamSession{manager: normalizeGRPCAddress(manager), token: token, tls: tlsCfg, next: 1}
}

func (s *ControlStreamSession) Open(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("control stream session is nil")
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
	stream, err := controlv1.NewAgentControlServiceClient(conn).ControlStream(ctx)
	if err != nil {
		_ = conn.Close()
		return err
	}
	s.conn = conn
	s.stream = stream
	return nil
}

func (s *ControlStreamSession) Hello(ctx context.Context, tenantID, agentID, scopeType, scopeSelector string) ([]*controlv1.ControlStreamFrame, error) {
	requestID := "hello-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if err := s.Send(ctx, &controlv1.ControlStreamFrame{
		Type:      "hello",
		RequestId: requestID,
		Context: &controlv1.RequestContext{
			TenantId: tenantID,
			AgentId:  agentID,
			Scope:    &controlv1.Scope{Type: scopeType, Selector: scopeSelector},
		},
	}); err != nil {
		return nil, err
	}
	var frames []*controlv1.ControlStreamFrame
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
			return nil, fmt.Errorf("control stream hello rejected: %s", frame.GetAck().GetMessage())
		}
		frames = append(frames, frame)
		if frame.GetType() == "resume" {
			return frames, nil
		}
	}
}

func (s *ControlStreamSession) ReportHealth(ctx context.Context, health agenthealth.AgentHealth) error {
	if err := s.SendHealth(ctx, health); err != nil {
		return err
	}
	reply, err := s.Recv()
	if err != nil {
		return err
	}
	if reply.GetType() != "ack" || reply.GetAck().GetStatus() != "accepted" {
		return fmt.Errorf("control stream health rejected: %s", reply.GetAck().GetMessage())
	}
	return nil
}

func (s *ControlStreamSession) SendHealth(ctx context.Context, health agenthealth.AgentHealth) error {
	return s.Send(ctx, &controlv1.ControlStreamFrame{
		Type:      "health_report",
		RequestId: "health-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		Context: &controlv1.RequestContext{
			TenantId: health.TenantID,
			AgentId:  health.AgentID,
			Scope:    &controlv1.Scope{Type: health.Scope.Type, Selector: health.Scope.Selector},
		},
		Health: healthResponse(health),
	})
}

func (s *ControlStreamSession) SendResponseAck(ctx context.Context, ack responsemodel.Ack) error {
	return s.Send(ctx, &controlv1.ControlStreamFrame{
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

func (s *ControlStreamSession) SendEvidenceResult(ctx context.Context, result gatewaymodel.EvidencePullbackResult) error {
	return s.Send(ctx, &controlv1.ControlStreamFrame{
		Type:      "evidence_pullback_result",
		RequestId: result.RequestID,
		Context:   &controlv1.RequestContext{TenantId: result.TenantID, AgentId: result.AgentID},
		EvidenceResult: &controlv1.EvidencePullbackResult{
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

func (s *ControlStreamSession) SendCapability(ctx context.Context, health agenthealth.AgentHealth) error {
	return s.Send(ctx, &controlv1.ControlStreamFrame{
		Type:      "capability_report",
		RequestId: "capability-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		Context: &controlv1.RequestContext{
			TenantId: health.TenantID,
			AgentId:  health.AgentID,
			Scope:    &controlv1.Scope{Type: health.Scope.Type, Selector: health.Scope.Selector},
		},
		Capability: capabilityResponse(health),
	})
}

func capabilityResponse(health agenthealth.AgentHealth) *controlv1.CapabilityResponse {
	return &controlv1.CapabilityResponse{
		AgentId:                  health.AgentID,
		HostId:                   health.HostID,
		TenantId:                 health.TenantID,
		Scope:                    scopeMessage(health.Scope),
		Sensor:                   capabilityMessage(health.Capability),
		SupportedPolicySections:  []string{"collection", "detection", "upload"},
		SupportedResponseActions: []string{"collect", "noop"},
		CollectionBehaviors:      collectionBehaviorMessages(health.Capability.Collection),
	}
}

func (s *ControlStreamSession) Send(ctx context.Context, frame *controlv1.ControlStreamFrame) error {
	if s == nil {
		return fmt.Errorf("control stream session is nil")
	}
	if frame == nil {
		return fmt.Errorf("control stream frame is nil")
	}
	if frame.GetRequestId() == "" {
		return fmt.Errorf("control stream request_id is required")
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

func (s *ControlStreamSession) Recv() (*controlv1.ControlStreamFrame, error) {
	if s == nil || s.stream == nil {
		return nil, fmt.Errorf("control stream session is not open")
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

func (s *ControlStreamSession) Close() error {
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
