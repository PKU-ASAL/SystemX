package daemon

import (
	"context"
	"fmt"
	"strings"

	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func (s *localControlServer) ApplyPolicy(ctx context.Context, req *controlplanev1.ApplyPolicyRequest) (*controlplanev1.ControlAck, error) {
	controller := newPolicyController(s.runner, s.runtime, s.batcher)
	return controlAck(controller.ApplyPolicy(ctx, policyCommand(req, agentcontrol.PolicySourceStandalone))), nil
}

func (s *localControlServer) ApplyContent(ctx context.Context, req *controlplanev1.ApplyContentRequest) (*controlplanev1.ControlAck, error) {
	controller := newContentController(s.runner)
	return controlAck(controller.ApplyContent(ctx, contentCommand(req, agentcontrol.PolicySourceStandalone))), nil
}

func (s *localControlServer) ListContent(ctx context.Context, req *controlplanev1.ListContentRequest) (*controlplanev1.ListContentResponse, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return nil, err
	}
	records, err := newContentController(s.runner).ListContent(ctx, req.GetKind())
	if err != nil {
		return nil, err
	}
	out := make([]*controlplanev1.ContentRecord, 0, len(records))
	for _, record := range records {
		out = append(out, contentRecordMessage(record))
	}
	return &controlplanev1.ListContentResponse{Records: out}, nil
}

func (s *localControlServer) GetContent(ctx context.Context, req *controlplanev1.GetContentRequest) (*controlplanev1.ContentGetResponse, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return nil, err
	}
	record, ok, err := newContentController(s.runner).GetContent(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("content ref %q not found", req.GetRef())
	}
	return &controlplanev1.ContentGetResponse{Record: contentRecordMessage(record)}, nil
}

func (s *localControlServer) GetEvent(ctx context.Context, req *controlplanev1.GetEventRequest) (*controlplanev1.EventGetResponse, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return nil, err
	}
	eventID := strings.TrimSpace(req.GetEventId())
	if eventID == "" {
		return nil, fmt.Errorf("event id is required")
	}
	frame, ok := s.eventFrameByID(eventID)
	if !ok {
		return nil, fmt.Errorf("event %q not found in telemetry buffer", eventID)
	}
	return &controlplanev1.EventGetResponse{Frame: frame}, nil
}
