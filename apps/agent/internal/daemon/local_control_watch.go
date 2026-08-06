package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/config"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/localstore"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/dataplane/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
)

func (s *localTelemetryService) GetEvent(ctx context.Context, req *controlplanev1.GetEventRequest) (*controlplanev1.EventGetResponse, error) {
	if err := s.runner.validateControlContext(req.GetContext()); err != nil {
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

func (s *localTelemetryService) WatchEvents(req *controlplanev1.WatchEventsRequest, stream controlplanev1.AgentControlPlaneService_WatchEventsServer) error {
	if err := s.runner.validateControlContext(req.GetContext()); err != nil {
		return err
	}
	sent := uint32(0)
	send := func(frame *controlplanev1.EventFrame) error {
		if !eventFrameMatches(frame, req.GetBehavior(), req.GetFilter()) {
			return nil
		}
		if err := stream.Send(frame); err != nil {
			return err
		}
		sent++
		return nil
	}
	if req.GetSnapshotOnly() {
		if req.GetIncludeRecent() {
			frames, err := s.recentEvents(req)
			if err != nil {
				return err
			}
			for _, frame := range frames {
				if err := send(controlEventFrame(s.runner.Config, frame)); err != nil {
					return err
				}
				if req.GetLimit() > 0 && sent >= req.GetLimit() {
					return nil
				}
			}
		}
		return nil
	}
	if req.GetIncludeRecent() {
		frames, err := s.recentEvents(req)
		if err != nil {
			return err
		}
		for _, frame := range frames {
			if err := send(controlEventFrame(s.runner.Config, frame)); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
	}
	entries := s.bus.WatchEvents(stream.Context())
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case entry, ok := <-entries:
			if !ok {
				return nil
			}
			if err := send(controlEventFrame(s.runner.Config, entry)); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
	}
}

func (s *localTelemetryService) WatchSignals(req *controlplanev1.WatchSignalsRequest, stream controlplanev1.AgentControlPlaneService_WatchSignalsServer) error {
	if err := s.runner.validateControlContext(req.GetContext()); err != nil {
		return err
	}
	sent := uint32(0)
	send := func(frame *controlplanev1.SignalFrame) error {
		if !signalFrameMatches(frame, req.GetRuleId(), req.GetWhere(), req.GetFilter()) {
			return nil
		}
		if err := stream.Send(frame); err != nil {
			return err
		}
		sent++
		return nil
	}
	if req.GetSnapshotOnly() {
		if req.GetIncludeRecent() {
			frames, err := s.recentSignals(req)
			if err != nil {
				return err
			}
			for _, frame := range frames {
				if err := send(controlSignalFrame(s.runner.Config, frame)); err != nil {
					return err
				}
				if req.GetLimit() > 0 && sent >= req.GetLimit() {
					return nil
				}
			}
		}
		return nil
	}
	if req.GetIncludeRecent() {
		frames, err := s.recentSignals(req)
		if err != nil {
			return err
		}
		for _, frame := range frames {
			if err := send(controlSignalFrame(s.runner.Config, frame)); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
	}
	entries := s.bus.WatchSignals(stream.Context())
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case entry, ok := <-entries:
			if !ok {
				return nil
			}
			if err := send(controlSignalFrame(s.runner.Config, entry)); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
	}
}

func (s *localTelemetryService) eventFrameByID(eventID string) (*controlplanev1.EventFrame, bool) {
	if s.runner.localStore != nil {
		frames, err := s.runner.localStore.QueryEvents(context.Background(), localstore.EventQuery{Limit: 1000})
		if err == nil {
			for _, frame := range frames {
				if frame.GetEvent().GetId() == eventID {
					return controlEventFrame(s.runner.Config, frame), true
				}
			}
		}
	}
	for _, frame := range s.bus.SnapshotEvents() {
		out := controlEventFrame(s.runner.Config, frame)
		if out.GetEvent().GetId() == eventID {
			return out, true
		}
	}
	return nil, false
}

func (s *localTelemetryService) recentEvents(req *controlplanev1.WatchEventsRequest) ([]*dataplanev1.EventFrame, error) {
	if s.runner.localStore == nil {
		return s.bus.SnapshotEvents(), nil
	}
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 100
	}
	return s.runner.localStore.QueryEvents(context.Background(), localstore.EventQuery{Behavior: req.GetBehavior(), AfterSequence: req.GetFilter().GetAfterSequence(), Limit: limit})
}

func (s *localTelemetryService) recentSignals(req *controlplanev1.WatchSignalsRequest) ([]*dataplanev1.SignalFrame, error) {
	if s.runner.localStore == nil {
		return s.bus.SnapshotSignals(), nil
	}
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 100
	}
	return s.runner.localStore.QuerySignals(context.Background(), localstore.SignalQuery{RuleID: req.GetRuleId(), Limit: limit})
}

func (s *localTelemetryService) watchAfterBatchID(filter *controlplanev1.WatchFilter, includeRecent bool) string {
	return ""
}

func controlEventFrame(cfg config.Config, frame *dataplanev1.EventFrame) *controlplanev1.EventFrame {
	if frame == nil {
		return &controlplanev1.EventFrame{TenantId: cfg.Agent.TenantID, AgentId: cfg.Agent.ID}
	}
	return &controlplanev1.EventFrame{
		TenantId:   cfg.Agent.TenantID,
		AgentId:    cfg.Agent.ID,
		Sequence:   frame.GetSequence(),
		ObservedAt: frame.GetObservedAt(),
		Event:      frame.GetEvent(),
	}
}

func controlSignalFrame(cfg config.Config, frame *dataplanev1.SignalFrame) *controlplanev1.SignalFrame {
	if frame == nil {
		return &controlplanev1.SignalFrame{TenantId: cfg.Agent.TenantID, AgentId: cfg.Agent.ID}
	}
	return &controlplanev1.SignalFrame{
		TenantId:   cfg.Agent.TenantID,
		AgentId:    cfg.Agent.ID,
		Sequence:   frame.GetSequence(),
		ObservedAt: frame.GetObservedAt(),
		Signal:     frame.GetSignal(),
	}
}

func eventMatches(event *eventv1.CanonicalEvent, behavior string) bool {
	if event == nil {
		return false
	}
	if behavior = strings.TrimSpace(strings.ToLower(behavior)); behavior != "" {
		return strings.TrimSpace(strings.ToLower(event.GetBehavior())) == behavior
	}
	return true
}

func eventFrameMatches(frame *controlplanev1.EventFrame, behavior string, filter *controlplanev1.WatchFilter) bool {
	if frame == nil || !eventMatches(frame.GetEvent(), behavior) {
		return false
	}
	return frameMatches(frame.GetSequence(), frame.GetObservedAt(), frame.GetEvent().GetLabels(), filter)
}

func signalMatches(signal *signalv1.Signal, ruleID, where string) bool {
	if signal == nil {
		return false
	}
	if ruleID = strings.TrimSpace(ruleID); ruleID != "" && signal.GetName() != ruleID {
		return false
	}
	where = strings.TrimSpace(strings.ToLower(where))
	if where == "" {
		return true
	}
	switch where {
	case "endpoint":
		return signal.GetWhere() == signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT
	case "cloud":
		return signal.GetWhere() == signalv1.SignalWhere_SIGNAL_WHERE_CLOUD
	default:
		return false
	}
}

func signalFrameMatches(frame *controlplanev1.SignalFrame, ruleID, where string, filter *controlplanev1.WatchFilter) bool {
	if frame == nil || !signalMatches(frame.GetSignal(), ruleID, where) {
		return false
	}
	return frameMatches(frame.GetSequence(), frame.GetObservedAt(), frame.GetSignal().GetLabels(), filter)
}

func frameMatches(sequence uint64, observedAt string, labels map[string]string, filter *controlplanev1.WatchFilter) bool {
	if filter == nil {
		return true
	}
	if after := filter.GetAfterSequence(); after > 0 && sequence <= after {
		return false
	}
	if !observedAtMatches(observedAt, filter.GetSinceObservedAt(), filter.GetUntilObservedAt()) {
		return false
	}
	for key, want := range filter.GetLabels() {
		if labels[key] != want {
			return false
		}
	}
	return true
}

func observedAtMatches(observedAt, since, until string) bool {
	if strings.TrimSpace(since) == "" && strings.TrimSpace(until) == "" {
		return true
	}
	ts, err := time.Parse(time.RFC3339Nano, observedAt)
	if err != nil {
		return false
	}
	if strings.TrimSpace(since) != "" {
		start, err := time.Parse(time.RFC3339Nano, since)
		if err != nil || ts.Before(start) {
			return false
		}
	}
	if strings.TrimSpace(until) != "" {
		end, err := time.Parse(time.RFC3339Nano, until)
		if err != nil || !ts.Before(end) {
			return false
		}
	}
	return true
}
