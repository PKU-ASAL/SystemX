package localapi

import (
	"context"
	"fmt"

	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

type StatusService interface {
	Health(context.Context, *controlplanev1.HealthRequest) (*controlplanev1.HealthResponse, error)
	Capability(context.Context, *controlplanev1.CapabilityRequest) (*controlplanev1.CapabilityResponse, error)
}

type TelemetryService interface {
	GetEvent(context.Context, *controlplanev1.GetEventRequest) (*controlplanev1.EventGetResponse, error)
	WatchEvents(*controlplanev1.WatchEventsRequest, controlplanev1.AgentControlPlaneService_WatchEventsServer) error
	WatchSignals(*controlplanev1.WatchSignalsRequest, controlplanev1.AgentControlPlaneService_WatchSignalsServer) error
}

type DebugService interface {
	DebugProfile(context.Context, *controlplanev1.DebugProfileRequest) (*controlplanev1.DebugProfileResponse, error)
}

type Dependencies struct {
	Status     StatusService
	Telemetry  TelemetryService
	Debug      DebugService
	Policy     agentcontrol.PolicyController
	Content    agentcontrol.ContentController
	Enrollment agentcontrol.EnrollmentController
	Validate   func(*controlplanev1.RequestContext) error
}

type Handler struct {
	controlplanev1.UnimplementedAgentControlPlaneServiceServer
	deps Dependencies
}

func NewHandler(deps Dependencies) *Handler {
	return &Handler{deps: deps}
}

func (h *Handler) Health(ctx context.Context, req *controlplanev1.HealthRequest) (*controlplanev1.HealthResponse, error) {
	if h.deps.Status == nil {
		return nil, fmt.Errorf("local api status handler is unavailable")
	}
	return h.deps.Status.Health(ctx, req)
}

func (h *Handler) Capability(ctx context.Context, req *controlplanev1.CapabilityRequest) (*controlplanev1.CapabilityResponse, error) {
	if h.deps.Status == nil {
		return nil, fmt.Errorf("local api status handler is unavailable")
	}
	return h.deps.Status.Capability(ctx, req)
}

func (h *Handler) CurrentPolicy(ctx context.Context, _ *controlplanev1.CurrentPolicyRequest) (*controlplanev1.CurrentPolicyResponse, error) {
	if h.deps.Policy == nil {
		return nil, fmt.Errorf("local api policy controller is unavailable")
	}
	snapshot, err := h.deps.Policy.CurrentPolicy(ctx)
	if err != nil {
		return nil, err
	}
	return currentPolicyMessage(snapshot), nil
}

func (h *Handler) ApplyPolicy(ctx context.Context, req *controlplanev1.ApplyPolicyRequest) (*controlplanev1.ControlAck, error) {
	if h.deps.Policy == nil {
		return nil, fmt.Errorf("local api policy controller is unavailable")
	}
	return controlAck(h.deps.Policy.ApplyPolicy(ctx, policyCommand(req))), nil
}

func (h *Handler) ApplyContent(ctx context.Context, req *controlplanev1.ApplyContentRequest) (*controlplanev1.ControlAck, error) {
	if h.deps.Content == nil {
		return nil, fmt.Errorf("local api content controller is unavailable")
	}
	return controlAck(h.deps.Content.ApplyContent(ctx, contentCommand(req))), nil
}

func (h *Handler) ListContent(ctx context.Context, req *controlplanev1.ListContentRequest) (*controlplanev1.ListContentResponse, error) {
	if err := h.validate(req.GetContext()); err != nil {
		return nil, err
	}
	if h.deps.Content == nil {
		return nil, fmt.Errorf("local api content controller is unavailable")
	}
	records, err := h.deps.Content.ListContent(ctx, req.GetKind())
	if err != nil {
		return nil, err
	}
	out := make([]*controlplanev1.ContentRecord, 0, len(records))
	for _, record := range records {
		out = append(out, contentRecordMessage(record))
	}
	return &controlplanev1.ListContentResponse{Records: out}, nil
}

func (h *Handler) GetContent(ctx context.Context, req *controlplanev1.GetContentRequest) (*controlplanev1.ContentGetResponse, error) {
	if err := h.validate(req.GetContext()); err != nil {
		return nil, err
	}
	if h.deps.Content == nil {
		return nil, fmt.Errorf("local api content controller is unavailable")
	}
	record, ok, err := h.deps.Content.GetContent(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("content ref %q not found", req.GetRef())
	}
	return &controlplanev1.ContentGetResponse{Record: contentRecordMessage(record)}, nil
}

func (h *Handler) GetEvent(ctx context.Context, req *controlplanev1.GetEventRequest) (*controlplanev1.EventGetResponse, error) {
	if h.deps.Telemetry == nil {
		return nil, fmt.Errorf("local api telemetry handler is unavailable")
	}
	return h.deps.Telemetry.GetEvent(ctx, req)
}

func (h *Handler) DebugProfile(ctx context.Context, req *controlplanev1.DebugProfileRequest) (*controlplanev1.DebugProfileResponse, error) {
	if h.deps.Debug == nil {
		return nil, fmt.Errorf("local api debug handler is unavailable")
	}
	return h.deps.Debug.DebugProfile(ctx, req)
}

func (h *Handler) WatchEvents(req *controlplanev1.WatchEventsRequest, stream controlplanev1.AgentControlPlaneService_WatchEventsServer) error {
	if h.deps.Telemetry == nil {
		return fmt.Errorf("local api telemetry handler is unavailable")
	}
	return h.deps.Telemetry.WatchEvents(req, stream)
}

func (h *Handler) WatchSignals(req *controlplanev1.WatchSignalsRequest, stream controlplanev1.AgentControlPlaneService_WatchSignalsServer) error {
	if h.deps.Telemetry == nil {
		return fmt.Errorf("local api telemetry handler is unavailable")
	}
	return h.deps.Telemetry.WatchSignals(req, stream)
}

func (h *Handler) Enroll(ctx context.Context, req *controlplanev1.EnrollRequest) (*controlplanev1.ControlAck, error) {
	if h.deps.Enrollment == nil {
		return nil, fmt.Errorf("local api enrollment controller is unavailable")
	}
	return controlAck(h.deps.Enrollment.Enroll(ctx, enrollmentCommand(req))), nil
}

func (h *Handler) Unenroll(ctx context.Context, req *controlplanev1.UnenrollRequest) (*controlplanev1.ControlAck, error) {
	if h.deps.Enrollment == nil {
		return nil, fmt.Errorf("local api enrollment controller is unavailable")
	}
	return controlAck(h.deps.Enrollment.Unenroll(ctx, unenrollmentCommand(req))), nil
}

func (h *Handler) validate(ctx *controlplanev1.RequestContext) error {
	if h.deps.Validate == nil {
		return nil
	}
	return h.deps.Validate(ctx)
}
