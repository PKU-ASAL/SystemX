package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agentcontent "github.com/sysarmor/sysarmor-next-project/internal/agent/content"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	agentpolicy "github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/uploadworker"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/detection"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
	"google.golang.org/grpc"
)

func (r *Runner) startLocalControlServer(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *uploadworker.Worker, startedAt time.Time) (func(), error) {
	socketPath := r.Config.Control.SocketPath
	if socketPath == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0o660); err != nil {
		_ = lis.Close()
		return nil, err
	}
	server := grpc.NewServer()
	controlv1.RegisterAgentControlServiceServer(server, &localControlServer{
		runner:    r,
		runtime:   rt,
		queue:     queue,
		worker:    worker,
		startedAt: startedAt,
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.Serve(lis); err != nil && r.Out != nil {
			fmt.Fprintf(r.Out, "agent local control server stopped: %v\n", err)
		}
	}()
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	return func() {
		server.GracefulStop()
		<-done
		_ = os.Remove(socketPath)
	}, nil
}

type localControlServer struct {
	controlv1.UnimplementedAgentControlServiceServer
	runner    *Runner
	runtime   sensorruntime.Runtime
	queue     *spool.Queue
	worker    *uploadworker.Worker
	startedAt time.Time
}

func (s *localControlServer) Health(ctx context.Context, req *controlv1.HealthRequest) (*controlv1.HealthResponse, error) {
	health, err := s.runner.collectHealth(ctx, s.runtime, s.queue, s.worker, s.startedAt)
	if err != nil {
		return nil, err
	}
	return healthResponse(health), nil
}

func (s *localControlServer) Capability(ctx context.Context, req *controlv1.CapabilityRequest) (*controlv1.CapabilityResponse, error) {
	cfg := s.runner.Config
	return &controlv1.CapabilityResponse{
		AgentId:  cfg.Agent.ID,
		HostId:   cfg.Agent.HostID,
		TenantId: cfg.Agent.TenantID,
		Scope:    scopeMessage(s.runner.runtimeScope()),
		Sensor:   capabilityMessage(s.runner.runtimeCapability()),
		SupportedPolicySections: []string{
			"collection",
			"detection",
			"response",
			"resource",
			"upload",
		},
		SupportedResponseActions: []string{
			"collect_evidence",
		},
		CollectionBehaviors: collectionBehaviorMessages(s.runner.runtimeCapability().Collection),
	}, nil
}

func (s *localControlServer) CurrentPolicy(ctx context.Context, req *controlv1.CurrentPolicyRequest) (*controlv1.CurrentPolicyResponse, error) {
	policy := policymodel.Normalize(s.runner.activePolicy())
	raw, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	return &controlv1.CurrentPolicyResponse{
		PolicyId: policy.PolicyID,
		Version:  policy.Version,
		TenantId: policy.TenantID,
		Scope: &controlv1.Scope{
			Type:     policy.Scope.Type,
			Selector: policy.Scope.Selector,
		},
		Mode:       policy.Mode,
		CloudRules: append([]string(nil), policy.CloudRules...),
		Published:  policy.Published,
		RawJson:    string(raw),
	}, nil
}

func (s *localControlServer) ApplyPolicy(ctx context.Context, req *controlv1.ApplyPolicyRequest) (*controlv1.ControlAck, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "policy", err.Error()), nil
	}
	policyType := strings.TrimSpace(req.GetPolicyType())
	if policyType == "" {
		policyType = "agent-runtime"
	}
	if policyType == "collection" {
		return s.applyCollectionPolicy(ctx, req), nil
	}
	if policyType == "detection" {
		return s.applyDetectionPolicy(req), nil
	}
	if policyType != "agent-runtime" {
		return rejectedAck(s.runner.Config, req.GetContext(), "policy", fmt.Sprintf("unsupported policy type %q", policyType)), nil
	}
	var next policymodel.Policy
	if err := json.Unmarshal([]byte(req.GetPolicyJson()), &next); err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "policy", "invalid policy json: "+err.Error()), nil
	}
	next = policymodel.Normalize(next)
	if next.TenantID == "" {
		next.TenantID = s.runner.Config.Agent.TenantID
	}
	if next.TenantID != "" && next.TenantID != s.runner.Config.Agent.TenantID {
		return rejectedAck(s.runner.Config, req.GetContext(), "policy", fmt.Sprintf("tenant mismatch: policy=%s agent=%s", next.TenantID, s.runner.Config.Agent.TenantID)), nil
	}
	if req.GetDryRun() {
		return appliedAck(s.runner.Config, req.GetContext(), next, "validated", "policy accepted in dry-run", false), nil
	}
	s.runner.applyRuntimePolicy(next)
	return appliedAck(s.runner.Config, req.GetContext(), next, "applied", "runtime policy applied", false), nil
}

func (s *localControlServer) ApplyContent(ctx context.Context, req *controlv1.ApplyContentRequest) (*controlv1.ControlAck, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "content", err.Error()), nil
	}
	record, err := s.runner.contentStore().Apply(req.GetContentJson(), req.GetAllowUnsigned(), req.GetDryRun())
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "content", err.Error()), nil
	}
	status := record.Status
	var report detection.ApplyReport
	if !req.GetDryRun() {
		report = s.runner.rebuildDetection()
		if report.Status == "degraded" {
			status = "degraded"
		}
	}
	message := fmt.Sprintf("content %s %s@%s digest=%s", status, record.Ref, record.Version, record.Digest)
	if len(report.Warnings) > 0 {
		message += "; detection dependencies degraded: " + strings.Join(report.Warnings, "; ")
	}
	return &controlv1.ControlAck{
		RequestId: requestID(req.GetContext()),
		TenantId:  s.runner.Config.Agent.TenantID,
		AgentId:   s.runner.Config.Agent.ID,
		Status:    status,
		Message:   message,
		PolicyId:  record.Ref,
		Sections: []*controlv1.AppliedSection{{
			Name:    "content",
			Status:  status,
			Message: message,
		}},
	}, nil
}

func (s *localControlServer) ListContent(ctx context.Context, req *controlv1.ListContentRequest) (*controlv1.ListContentResponse, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return nil, err
	}
	records := s.runner.contentStore().List(strings.TrimSpace(req.GetKind()))
	out := make([]*controlv1.ContentRecord, 0, len(records))
	for _, record := range records {
		out = append(out, contentRecordMessage(record))
	}
	return &controlv1.ListContentResponse{Records: out}, nil
}

func (s *localControlServer) GetContent(ctx context.Context, req *controlv1.GetContentRequest) (*controlv1.ContentGetResponse, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return nil, err
	}
	record, ok := s.runner.contentStore().Get(strings.TrimSpace(req.GetRef()))
	if !ok {
		return nil, fmt.Errorf("content ref %q not found", req.GetRef())
	}
	return &controlv1.ContentGetResponse{Record: contentRecordMessage(record)}, nil
}

func (s *localControlServer) GetEvent(ctx context.Context, req *controlv1.GetEventRequest) (*controlv1.EventGetResponse, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return nil, err
	}
	eventID := strings.TrimSpace(req.GetEventId())
	if eventID == "" {
		return nil, fmt.Errorf("event id is required")
	}
	frame, ok := s.runner.localStreams().getEvent(eventID)
	if !ok {
		return nil, fmt.Errorf("event %q not found in recent buffer", eventID)
	}
	return &controlv1.EventGetResponse{Frame: frame}, nil
}

func (s *localControlServer) applyCollectionPolicy(ctx context.Context, req *controlv1.ApplyPolicyRequest) *controlv1.ControlAck {
	policy, err := agentpolicy.ParseCollectionPolicyJSON([]byte(req.GetPolicyJson()), s.runner.Config.Sensor.ObserveOnly)
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "collection", "invalid collection policy: "+err.Error())
	}
	reqScope := requestScope(req.GetContext())
	if policy.ScopeType == "" && reqScope.Type != "" {
		policy.ScopeType = reqScope.Type
		policy.ScopeSelector = reqScope.Selector
	}
	if policy.ScopeType == "" {
		if scope, err := s.runner.Config.Sensor.EffectiveScope(); err == nil {
			policy.ScopeType = scope.Type
			policy.ScopeSelector = scope.Selector
		}
	}
	intent, err := agentpolicy.CollectionPolicyIntent(policy)
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "collection", "compile collection policy: "+err.Error())
	}
	if req.GetDryRun() {
		return collectionAck(s.runner.Config, req.GetContext(), policy, "validated", "collection policy accepted in dry-run", false)
	}
	if err := s.runtime.Apply(ctx, intent); err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "collection", "apply collection policy: "+err.Error())
	}
	s.runner.setCollectionIntent(intent)
	active := policymodel.Normalize(s.runner.activePolicy())
	engine, report := detection.NewWithRuntimeLimits(active.Detection, intent, s.runner.detectionContentSnapshot(), s.runner.detectionLimits())
	s.runner.setDetection(engine)
	if report.Status == "degraded" {
		return collectionAck(s.runner.Config, req.GetContext(), policy, "degraded", "collection policy applied; detection dependencies degraded: "+strings.Join(report.Warnings, "; "), false)
	}
	return collectionAck(s.runner.Config, req.GetContext(), policy, "applied", "collection policy applied", false)
}

func (s *localControlServer) applyDetectionPolicy(req *controlv1.ApplyPolicyRequest) *controlv1.ControlAck {
	var envelope struct {
		Detection *policymodel.DetectionPolicy `json:"detection"`
	}
	var next policymodel.DetectionPolicy
	if err := json.Unmarshal([]byte(req.GetPolicyJson()), &envelope); err == nil && envelope.Detection != nil {
		next = *envelope.Detection
	} else if err := json.Unmarshal([]byte(req.GetPolicyJson()), &next); err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "detection", "invalid detection policy json: "+err.Error())
	}
	next = policymodel.NormalizeDetectionPolicy(next)
	engine, report := detection.NewWithRuntimeLimits(&next, s.runner.currentCollectionIntent(), s.runner.detectionContentSnapshot(), s.runner.detectionLimits())
	active := policymodel.Normalize(s.runner.activePolicy())
	active.Detection = &next
	if req.GetDryRun() {
		return detectionAck(s.runner.Config, req.GetContext(), active, report.Status, "detection policy accepted in dry-run: "+report.Message, false, report)
	}
	s.runner.setPolicy(active)
	s.runner.setDetection(engine)
	return detectionAck(s.runner.Config, req.GetContext(), active, report.Status, report.Message, false, report)
}

func (s *localControlServer) WatchEvents(req *controlv1.WatchEventsRequest, stream controlv1.AgentControlService_WatchEventsServer) error {
	if err := s.validateContext(req.GetContext()); err != nil {
		return err
	}
	sent := uint32(0)
	send := func(frame *controlv1.EventFrame) error {
		if !eventMatches(frame.GetEvent(), req.GetBehavior()) {
			return nil
		}
		if err := stream.Send(frame); err != nil {
			return err
		}
		sent++
		return nil
	}
	if req.GetIncludeRecent() {
		for _, frame := range s.runner.localStreams().recentEvents() {
			if err := send(frame); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
	}
	if req.GetSnapshotOnly() {
		return nil
	}
	ch, unsubscribe := s.runner.localStreams().subscribeEvents()
	defer unsubscribe()
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case frame, ok := <-ch:
			if !ok {
				return nil
			}
			if err := send(frame); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
	}
}

func (s *localControlServer) WatchSignals(req *controlv1.WatchSignalsRequest, stream controlv1.AgentControlService_WatchSignalsServer) error {
	if err := s.validateContext(req.GetContext()); err != nil {
		return err
	}
	sent := uint32(0)
	send := func(frame *controlv1.SignalFrame) error {
		if !signalMatches(frame.GetSignal(), req.GetRuleId(), req.GetWhere()) {
			return nil
		}
		if err := stream.Send(frame); err != nil {
			return err
		}
		sent++
		return nil
	}
	if req.GetIncludeRecent() {
		for _, frame := range s.runner.localStreams().recentSignals() {
			if err := send(frame); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
	}
	if req.GetSnapshotOnly() {
		return nil
	}
	ch, unsubscribe := s.runner.localStreams().subscribeSignals()
	defer unsubscribe()
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case frame, ok := <-ch:
			if !ok {
				return nil
			}
			if err := send(frame); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
	}
}

func (s *localControlServer) validateContext(ctx *controlv1.RequestContext) error {
	if ctx == nil {
		return nil
	}
	if tenantID := strings.TrimSpace(ctx.GetTenantId()); tenantID != "" && tenantID != s.runner.Config.Agent.TenantID {
		return fmt.Errorf("tenant mismatch: request=%s agent=%s", tenantID, s.runner.Config.Agent.TenantID)
	}
	if agentID := strings.TrimSpace(ctx.GetAgentId()); agentID != "" && agentID != s.runner.Config.Agent.ID {
		return fmt.Errorf("agent mismatch: request=%s agent=%s", agentID, s.runner.Config.Agent.ID)
	}
	return nil
}

func rejectedAck(cfg config.Config, req *controlv1.RequestContext, section, message string) *controlv1.ControlAck {
	return &controlv1.ControlAck{
		RequestId: requestID(req),
		TenantId:  cfg.Agent.TenantID,
		AgentId:   cfg.Agent.ID,
		Status:    "rejected",
		Message:   message,
		Sections: []*controlv1.AppliedSection{{
			Name:    section,
			Status:  "rejected",
			Message: message,
		}},
	}
}

func appliedAck(cfg config.Config, req *controlv1.RequestContext, policy policymodel.Policy, status, message string, requiresRestart bool) *controlv1.ControlAck {
	return &controlv1.ControlAck{
		RequestId:     requestID(req),
		TenantId:      cfg.Agent.TenantID,
		AgentId:       cfg.Agent.ID,
		Status:        status,
		Message:       message,
		PolicyId:      policy.PolicyID,
		PolicyVersion: policy.Version,
		Sections: []*controlv1.AppliedSection{
			{Name: "detection", Status: status, Message: "endpoint rules updated", RequiresRestart: false},
			{Name: "response", Status: status, Message: "response policy updated", RequiresRestart: false},
			{Name: "resource", Status: "unsupported", Message: "resource policy contract is reserved for the next phase", RequiresRestart: requiresRestart},
			{Name: "upload", Status: "unsupported", Message: "upload policy contract is reserved for the next phase", RequiresRestart: requiresRestart},
			{Name: "collection", Status: "unsupported", Message: "collection hot reload requires compiler/runtime apply in the next phase", RequiresRestart: true},
		},
	}
}

func requestID(req *controlv1.RequestContext) string {
	if req == nil {
		return ""
	}
	return req.GetRequestId()
}

func requestScope(req *controlv1.RequestContext) config.RuntimeScope {
	if req == nil || req.GetScope() == nil {
		return config.RuntimeScope{}
	}
	return config.RuntimeScope{Type: req.GetScope().GetType(), Selector: req.GetScope().GetSelector()}
}

func collectionAck(cfg config.Config, req *controlv1.RequestContext, policy agentpolicy.CollectionPolicy, status, message string, requiresRestart bool) *controlv1.ControlAck {
	return &controlv1.ControlAck{
		RequestId:     requestID(req),
		TenantId:      cfg.Agent.TenantID,
		AgentId:       cfg.Agent.ID,
		Status:        status,
		Message:       message,
		PolicyId:      policy.PolicyID,
		PolicyVersion: policy.Version,
		Sections: []*controlv1.AppliedSection{{
			Name:            "collection",
			Status:          status,
			Message:         message,
			RequiresRestart: requiresRestart,
		}},
	}
}

func detectionAck(cfg config.Config, req *controlv1.RequestContext, policy policymodel.Policy, status, message string, requiresRestart bool, report detection.ApplyReport) *controlv1.ControlAck {
	if status == "" {
		status = "applied"
	}
	if message == "" {
		message = "detection policy applied"
	}
	sectionMessage := message
	if len(report.Details) > 0 {
		sectionMessage = sectionMessage + ": " + strings.Join(report.Details, "; ")
	}
	return &controlv1.ControlAck{
		RequestId:     requestID(req),
		TenantId:      cfg.Agent.TenantID,
		AgentId:       cfg.Agent.ID,
		Status:        status,
		Message:       sectionMessage,
		PolicyId:      policy.PolicyID,
		PolicyVersion: policy.Version,
		Sections: []*controlv1.AppliedSection{{
			Name:            "detection",
			Status:          status,
			Message:         sectionMessage,
			RequiresRestart: requiresRestart,
		}},
	}
}

func contentRecordMessage(record agentcontent.Record) *controlv1.ContentRecord {
	return &controlv1.ContentRecord{
		Ref:     record.Ref,
		Kind:    record.Kind,
		Version: record.Version,
		Digest:  record.Digest,
		Signed:  record.Signed,
		Status:  record.Status,
		RawJson: record.RawJSON,
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

func healthResponse(health agenthealth.AgentHealth) *controlv1.HealthResponse {
	return &controlv1.HealthResponse{
		AgentId:       health.AgentID,
		HostId:        health.HostID,
		TenantId:      health.TenantID,
		Scope:         scopeMessage(health.Scope),
		Status:        health.Status,
		PolicyId:      health.PolicyID,
		PolicyVersion: health.PolicyVersion,
		PolicyMode:    health.PolicyMode,
		UptimeSeconds: health.UptimeSeconds,
		Capability:    capabilityMessage(health.Capability),
		Sensor: &controlv1.SensorHealth{
			Backend:        health.Sensor.Backend,
			Installed:      health.Sensor.Installed,
			Running:        health.Sensor.Running,
			Version:        health.Sensor.Version,
			PolicyLoaded:   health.Sensor.PolicyLoaded,
			EventsSeen:     health.Sensor.EventsSeen,
			EventsDropped:  health.Sensor.EventsDropped,
			ParseErrors:    health.Sensor.ParseErrors,
			RestartCount:   health.Sensor.RestartCount,
			LastEventAt:    timestampString(health.Sensor.LastEventAt),
			LastExitReason: health.Sensor.LastExitReason,
			LastError:      health.Sensor.LastError,
		},
		Queue: &controlv1.QueueHealth{
			QueuedBatches:     uint32(health.Queue.QueuedBatches),
			QueuedBytes:       health.Queue.QueuedBytes,
			MaxBytes:          health.Queue.MaxBytes,
			BackpressureCount: health.Queue.BackpressureCount,
			DroppedBatches:    health.Queue.DroppedBatches,
			DroppedBytes:      health.Queue.DroppedBytes,
			LastError:         health.Queue.LastError,
		},
		Upload: &controlv1.UploadHealth{
			UploadedBatches:  uint32(health.Upload.UploadedBatches),
			RemainingBatches: uint32(health.Upload.RemainingBatches),
			RemainingBytes:   health.Upload.RemainingBytes,
			LastError:        health.Upload.LastError,
		},
		Cep: &controlv1.CEPHealth{
			ActiveGroups:     health.CEP.ActiveGroups,
			EvictedGroups:    health.CEP.EvictedGroups,
			ExpiredGroups:    health.CEP.ExpiredGroups,
			DroppedEventRefs: health.CEP.DroppedEventRefs,
			EvalErrors:       health.CEP.EvalErrors,
			EmittedSignals:   health.CEP.EmittedSignals,
			Degraded:         health.CEP.Degraded,
		},
		ObservedAt: timestampString(health.ObservedAt),
	}
}

func scopeMessage(scope agenthealth.RuntimeScope) *controlv1.Scope {
	return &controlv1.Scope{Type: scope.Type, Selector: scope.Selector}
}

func capabilityMessage(cap agenthealth.SensorCapability) *controlv1.SensorCapability {
	return &controlv1.SensorCapability{
		Backend:         cap.Backend,
		Version:         cap.Version,
		SupportsExec:    cap.SupportsExec,
		SupportsConnect: cap.SupportsConnect,
		SupportsFile:    cap.SupportsFile,
		SupportsEnforce: cap.SupportsEnforce,
		SupportsHealth:  cap.SupportsHealth,
		KernelRelease:   cap.KernelRelease,
		BtfAvailable:    cap.BTFAvailable,
		BpffsAvailable:  cap.BPFFSAvailable,
	}
}

func collectionBehaviorMessages(in []agenthealth.CollectionBehaviorCapability) []*controlv1.CollectionBehaviorCapability {
	out := make([]*controlv1.CollectionBehaviorCapability, 0, len(in))
	for _, item := range in {
		out = append(out, &controlv1.CollectionBehaviorCapability{
			Behavior:             item.Behavior,
			Fields:               append([]string(nil), item.Fields...),
			PushdownSelectors:    append([]string(nil), item.PushdownSelectors...),
			AgentSideSelectors:   append([]string(nil), item.AgentSideSelectors...),
			UnsupportedSelectors: append([]string(nil), item.UnsupportedSelectors...),
			SensorMapping:        item.SensorMapping,
		})
	}
	return out
}

func timestampString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
