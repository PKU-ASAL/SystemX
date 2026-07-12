package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agentcontent "github.com/sysarmor/sysarmor-next-project/internal/agent/content"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
	agentpolicy "github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/telemetry"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/detection"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/sensors/contract"
	"github.com/sysarmor/sysarmor-next-project/internal/sensors/linux/tetragon"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensors/runtime"
	"google.golang.org/grpc"
)

func (r *AgentRuntime) startLocalControlServer(ctx context.Context, rt sensorruntime.Runtime, source any, rest ...any) (func(), error) {
	bus, batcher, sender, startedAt := r.localControlTelemetryArgs(source, rest...)
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
	controlplanev1.RegisterAgentControlPlaneServiceServer(server, &localControlServer{
		runner:    r,
		runtime:   rt,
		bus:       bus,
		batcher:   batcher,
		sender:    sender,
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

func (r *AgentRuntime) localControlTelemetryArgs(source any, rest ...any) (*telemetry.Bus, *telemetry.Batcher, *telemetry.Sender, time.Time) {
	if bus, ok := source.(*telemetry.Bus); ok {
		var batcher *telemetry.Batcher
		var sender *telemetry.Sender
		var startedAt time.Time
		if len(rest) > 0 {
			batcher, _ = rest[0].(*telemetry.Batcher)
		}
		if len(rest) > 1 {
			sender, _ = rest[1].(*telemetry.Sender)
		}
		if len(rest) > 2 {
			startedAt, _ = rest[2].(time.Time)
		}
		if batcher == nil {
			batcher = telemetry.NewBatcher(r.newDataBatch, r.Config.Telemetry.BatchSize, r.Config.Telemetry.FlushInterval, 64, r.Config.Telemetry.MaxBytes)
		}
		if sender == nil {
			sender = &telemetry.Sender{Appender: localBatchSender{}, Batcher: batcher}
		}
		if sender.Batcher == nil {
			sender.Batcher = batcher
		}
		if startedAt.IsZero() {
			startedAt = time.Now().UTC()
		}
		return bus, batcher, sender, startedAt
	}
	bus := telemetry.NewBus(r.Config.Telemetry.BatchSize * 16)
	batcher := telemetry.NewBatcher(r.newDataBatch, r.Config.Telemetry.BatchSize, r.Config.Telemetry.FlushInterval, 64, r.Config.Telemetry.MaxBytes)
	sender := &telemetry.Sender{Appender: localBatchSender{}, Batcher: batcher}
	startedAt := time.Now().UTC()
	return bus, batcher, sender, startedAt
}

type localControlServer struct {
	controlplanev1.UnimplementedAgentControlPlaneServiceServer
	runner    *AgentRuntime
	runtime   sensorruntime.Runtime
	bus       *telemetry.Bus
	batcher   *telemetry.Batcher
	sender    *telemetry.Sender
	startedAt time.Time
	profileMu sync.Mutex
}

func (s *localControlServer) Health(ctx context.Context, req *controlplanev1.HealthRequest) (*controlplanev1.HealthResponse, error) {
	health, err := s.runner.collectHealth(ctx, s.runtime, s.bus, s.batcher, s.sender, s.startedAt)
	if err != nil {
		return nil, err
	}
	response := healthResponse(health)
	if s.runner.localStore != nil {
		response.LocalStore, err = s.runner.localStoreHealth(ctx)
	}
	return response, err
}

func (s *localControlServer) Capability(ctx context.Context, req *controlplanev1.CapabilityRequest) (*controlplanev1.CapabilityResponse, error) {
	cfg := s.runner.Config
	return &controlplanev1.CapabilityResponse{
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
			"data_plane",
		},
		SupportedResponseActions: []string{
			"collect_evidence",
		},
		CollectionBehaviors: collectionBehaviorMessages(s.runner.runtimeCapability().Collection),
	}, nil
}

func (s *localControlServer) DebugProfile(ctx context.Context, req *controlplanev1.DebugProfileRequest) (*controlplanev1.DebugProfileResponse, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return nil, err
	}
	profileType := strings.TrimSpace(req.GetProfileType())
	if profileType == "" {
		profileType = "cpu"
	}
	switch profileType {
	case "cpu", "heap", "allocs", "goroutine", "threadcreate", "block", "mutex", "runtime":
	default:
		return nil, fmt.Errorf("unsupported debug profile type %q", profileType)
	}
	seconds := req.GetSeconds()
	if seconds == 0 {
		seconds = 10
	}
	if seconds > 300 {
		return nil, fmt.Errorf("debug profile seconds must be <= 300")
	}
	if !s.profileMu.TryLock() {
		return nil, fmt.Errorf("debug profile already running")
	}
	defer s.profileMu.Unlock()

	var buf bytes.Buffer
	started := time.Now().UTC()
	switch profileType {
	case "cpu":
		if err := pprof.StartCPUProfile(&buf); err != nil {
			return nil, fmt.Errorf("start cpu profile: %w", err)
		}
		timer := time.NewTimer(time.Duration(seconds) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			pprof.StopCPUProfile()
			return nil, ctx.Err()
		case <-timer.C:
		}
		pprof.StopCPUProfile()
	case "runtime":
		stats := runtimeStatsPayload(started, strings.TrimSpace(req.GetLabel()))
		if err := json.NewEncoder(&buf).Encode(stats); err != nil {
			return nil, fmt.Errorf("encode runtime stats: %w", err)
		}
	default:
		runtime.GC()
		prof := pprof.Lookup(profileType)
		if prof == nil {
			return nil, fmt.Errorf("profile %q unavailable", profileType)
		}
		if err := prof.WriteTo(&buf, 0); err != nil {
			return nil, fmt.Errorf("write %s profile: %w", profileType, err)
		}
	}
	finished := time.Now().UTC()
	return &controlplanev1.DebugProfileResponse{
		ProfileType: profileType,
		Seconds:     seconds,
		StartedAt:   started.Format(time.RFC3339Nano),
		FinishedAt:  finished.Format(time.RFC3339Nano),
		Profile:     buf.Bytes(),
		Label:       strings.TrimSpace(req.GetLabel()),
	}, nil
}

func runtimeStatsPayload(observedAt time.Time, label string) map[string]any {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return map[string]any{
		"observed_at":         observedAt.Format(time.RFC3339Nano),
		"label":               label,
		"go_version":          runtime.Version(),
		"goos":                runtime.GOOS,
		"goarch":              runtime.GOARCH,
		"gomaxprocs":          runtime.GOMAXPROCS(0),
		"goroutines":          runtime.NumGoroutine(),
		"cgo_calls":           runtime.NumCgoCall(),
		"heap_alloc_bytes":    mem.HeapAlloc,
		"heap_sys_bytes":      mem.HeapSys,
		"heap_idle_bytes":     mem.HeapIdle,
		"heap_inuse_bytes":    mem.HeapInuse,
		"heap_released_bytes": mem.HeapReleased,
		"heap_objects":        mem.HeapObjects,
		"stack_inuse_bytes":   mem.StackInuse,
		"stack_sys_bytes":     mem.StackSys,
		"alloc_bytes_total":   mem.TotalAlloc,
		"mallocs_total":       mem.Mallocs,
		"frees_total":         mem.Frees,
		"gc_count":            mem.NumGC,
		"gc_pause_ns_total":   mem.PauseTotalNs,
		"last_gc_unix_ns":     mem.LastGC,
		"next_gc_bytes":       mem.NextGC,
		"gc_cpu_fraction":     mem.GCCPUFraction,
	}
}

func (s *localControlServer) CurrentPolicy(ctx context.Context, req *controlplanev1.CurrentPolicyRequest) (*controlplanev1.CurrentPolicyResponse, error) {
	policy := policymodel.Normalize(s.runner.activePolicy())
	raw, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	return &controlplanev1.CurrentPolicyResponse{
		PolicyId: policy.PolicyID,
		Version:  policy.Version,
		TenantId: policy.TenantID,
		Scope: &controlplanev1.Scope{
			Type:     policy.Scope.Type,
			Selector: policy.Scope.Selector,
		},
		Mode:       policy.Mode,
		CloudRules: append([]string(nil), policy.CloudRules...),
		Published:  policy.Published,
		RawJson:    string(raw),
	}, nil
}

func (s *localControlServer) ApplyPolicy(ctx context.Context, req *controlplanev1.ApplyPolicyRequest) (*controlplanev1.ControlAck, error) {
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
	if policyType == "data_plane" {
		return s.applyDataPlanePolicy(req, nil), nil
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
	dataPlaneSection, err := dataPlanePolicyFromRequest(req, next.DataPlane)
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "data_plane", err.Error()), nil
	}
	if dataPlaneSection != nil {
		next.DataPlane = dataPlaneSection
	}
	if req.GetDryRun() {
		return appliedAck(s.runner.Config, req.GetContext(), next, "validated", "policy accepted in dry-run", dataPlaneSection != nil), nil
	}
	report, ok := s.runner.tryApplyRuntimePolicy(next)
	if !ok {
		return rejectedAck(s.runner.Config, req.GetContext(), "detection", "runtime policy rejected; detection rebuild failed: "+strings.Join(report.Details, "; ")), nil
	}
	if dataPlaneSection != nil {
		s.runner.applyDataPlaneConfig(*dataPlaneSection)
	}
	return appliedAck(s.runner.Config, req.GetContext(), next, "applied", "runtime policy applied", dataPlaneSection != nil), nil
}

func (s *localControlServer) applyDataPlanePolicy(req *controlplanev1.ApplyPolicyRequest, fallback *policymodel.DataPlanePolicy) *controlplanev1.ControlAck {
	if fallback == nil && strings.TrimSpace(req.GetPolicyJson()) != "" {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(req.GetPolicyJson()), &raw); err != nil {
			return rejectedAck(s.runner.Config, req.GetContext(), "data_plane", "invalid data plane policy json: "+err.Error())
		}
		payload := []byte(req.GetPolicyJson())
		if nested, ok := raw["data_plane"]; ok {
			payload = nested
		}
		var dataPlane policymodel.DataPlanePolicy
		if err := json.Unmarshal(payload, &dataPlane); err != nil {
			return rejectedAck(s.runner.Config, req.GetContext(), "data_plane", "invalid data plane policy json: "+err.Error())
		}
		fallback = &dataPlane
	}
	dataPlane, err := dataPlanePolicyFromRequest(req, fallback)
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "data_plane", err.Error())
	}
	if dataPlane == nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "data_plane", "data plane policy is required")
	}
	if req.GetDryRun() {
		return dataPlaneAck(s.runner.Config, req.GetContext(), "validated", "data plane policy accepted in dry-run", true, *dataPlane)
	}
	s.runner.applyDataPlaneConfig(*dataPlane)
	return dataPlaneAck(s.runner.Config, req.GetContext(), "applied", "data plane policy applied; restart data batch dispatcher to take effect", true, *dataPlane)
}

func (s *localControlServer) ApplyContent(ctx context.Context, req *controlplanev1.ApplyContentRequest) (*controlplanev1.ControlAck, error) {
	return s.runner.applyContentUpdate(req), nil
}

func (r *AgentRuntime) applyPolicyUpdateFromControl(frame *controlplanev1.ControlFrame) *controlplanev1.ControlAck {
	ctx := frame.GetContext()
	if ctx == nil {
		ctx = &controlplanev1.RequestContext{}
	}
	if ctx.RequestId == "" {
		ctx.RequestId = frame.GetRequestId()
	}
	if err := r.validateControlContext(ctx); err != nil {
		return rejectedAck(r.Config, ctx, "policy", err.Error())
	}
	policy, err := policyFromControlFrame(frame.GetPolicyUpdate())
	if err != nil {
		return rejectedAck(r.Config, ctx, "policy", err.Error())
	}
	if policy.TenantID == "" {
		policy.TenantID = r.Config.Agent.TenantID
	}
	if policy.TenantID != "" && policy.TenantID != r.Config.Agent.TenantID {
		return rejectedAck(r.Config, ctx, "policy", fmt.Sprintf("tenant mismatch: policy=%s agent=%s", policy.TenantID, r.Config.Agent.TenantID))
	}
	if samePolicyRuntime(r.activePolicy(), policy) {
		return appliedAck(r.Config, ctx, policy, "applied", "runtime policy already active", false)
	}
	report, ok := r.tryApplyRuntimePolicy(policy)
	if !ok {
		return rejectedAck(r.Config, ctx, "detection", "runtime policy rejected; detection rebuild failed: "+strings.Join(report.Details, "; "))
	}
	if policy.DataPlane != nil {
		r.applyDataPlaneConfig(*policy.DataPlane)
	}
	message := "runtime policy applied"
	if report.Status == "degraded" {
		message = "runtime policy applied; detection dependencies degraded: " + strings.Join(report.Warnings, "; ")
	}
	return appliedAck(r.Config, ctx, policy, "applied", message, policy.DataPlane != nil)
}

func (r *AgentRuntime) applyContentUpdate(req *controlplanev1.ApplyContentRequest) *controlplanev1.ControlAck {
	if req == nil {
		return rejectedAck(r.Config, nil, "content", "content update request is required")
	}
	if err := r.validateControlContext(req.GetContext()); err != nil {
		return rejectedAck(r.Config, req.GetContext(), "content", err.Error())
	}
	var report detection.ApplyReport
	record, err := r.contentStore().Apply(req.GetContentJson(), req.GetAllowUnsigned(), true)
	if err != nil {
		return rejectedAck(r.Config, req.GetContext(), "content", err.Error())
	}
	status := record.Status
	if req.GetDryRun() {
		status = "validated"
	} else {
		var snapshot agentcontent.Snapshot
		record, snapshot, err := r.contentStore().Prepare(req.GetContentJson(), req.GetAllowUnsigned())
		if err != nil {
			return rejectedAck(r.Config, req.GetContext(), "content", err.Error())
		}
		var engine *detection.Engine
		engine, report = r.buildDetectionWithSnapshot(snapshot)
		if report.Status == "rejected" {
			message := "content rejected; detection rebuild failed: " + strings.Join(report.Details, "; ")
			r.setDetectionStatus(r.activePolicy(), report, r.contentStore().Snapshot())
			return rejectedAck(r.Config, req.GetContext(), "content", message)
		}
		if err := r.commitDetectionContent(record, snapshot, engine, report); err != nil {
			return rejectedAck(r.Config, req.GetContext(), "content", err.Error())
		}
		status = record.Status
		if report.Status == "degraded" {
			status = "degraded"
		}
	}
	message := fmt.Sprintf("content %s %s@%s digest=%s", status, record.Ref, record.Version, record.Digest)
	if len(report.Warnings) > 0 {
		message += "; detection dependencies degraded: " + strings.Join(report.Warnings, "; ")
	}
	return &controlplanev1.ControlAck{
		RequestId: requestID(req.GetContext()),
		TenantId:  r.Config.Agent.TenantID,
		AgentId:   r.Config.Agent.ID,
		Status:    status,
		Message:   message,
		PolicyId:  record.Ref,
		Sections: []*controlplanev1.AppliedSection{{
			Name:    "content",
			Status:  status,
			Message: message,
		}},
	}
}

func (s *localControlServer) ListContent(ctx context.Context, req *controlplanev1.ListContentRequest) (*controlplanev1.ListContentResponse, error) {
	if err := s.validateContext(req.GetContext()); err != nil {
		return nil, err
	}
	records := s.runner.contentStore().List(strings.TrimSpace(req.GetKind()))
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
	record, ok := s.runner.contentStore().Get(strings.TrimSpace(req.GetRef()))
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

func (s *localControlServer) applyCollectionPolicy(ctx context.Context, req *controlplanev1.ApplyPolicyRequest) *controlplanev1.ControlAck {
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
	policy, expansionReport, err := agentpolicy.ExpandCollectionPolicyRefs(policy, collectionContentSnapshot(s.runner.contentStore().Snapshot()))
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "collection", "resolve collection policy refs: "+err.Error())
	}
	intent, err := agentpolicy.CollectionPolicyIntent(policy)
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "collection", "compile collection policy: "+err.Error())
	}
	compileReport := tetragon.CompileReport(intent)
	compileReport.ResolvedRefs = expansionReport.ResolvedRefs
	if len(compileReport.UnsupportedSelectors) > 0 {
		return collectionAck(s.runner.Config, req.GetContext(), policy, "rejected", "collection policy contains unsupported selectors", false, compileReport, nil)
	}
	_, detectionReport := detection.NewWithRuntimeLimits(s.runner.activePolicy().Detection, s.runner.withCollectionCapabilities(intent), s.runner.detectionContentSnapshot(), s.runner.detectionLimits())
	if req.GetDryRun() {
		status := "validated"
		message := "collection policy accepted in dry-run"
		if detectionReport.Status == "degraded" {
			status = "degraded"
			message = "collection policy accepted in dry-run; detection dependencies degraded: " + strings.Join(detectionReport.Warnings, "; ")
		}
		return collectionAck(s.runner.Config, req.GetContext(), policy, status, message, false, compileReport, &detectionReport.Coverage)
	}
	if err := s.runtime.Apply(ctx, intent); err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "collection", "apply collection policy: "+err.Error())
	}
	s.runner.setCollectionIntent(intent)
	active := policymodel.Normalize(s.runner.activePolicy())
	engine, report := detection.NewWithRuntimeLimits(active.Detection, intent, s.runner.detectionContentSnapshot(), s.runner.detectionLimits())
	s.runner.setDetection(engine)
	if report.Status == "degraded" {
		return collectionAck(s.runner.Config, req.GetContext(), policy, "degraded", "collection policy applied; detection dependencies degraded: "+strings.Join(report.Warnings, "; "), false, compileReport, &report.Coverage)
	}
	return collectionAck(s.runner.Config, req.GetContext(), policy, "applied", "collection policy applied", false, compileReport, &report.Coverage)
}

func (s *localControlServer) applyDetectionPolicy(req *controlplanev1.ApplyPolicyRequest) *controlplanev1.ControlAck {
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
	if report.Status == "rejected" {
		s.runner.setDetectionStatus(active, report, s.runner.contentStore().Snapshot())
		return detectionAck(s.runner.Config, req.GetContext(), active, "rejected", "detection policy rejected: "+strings.Join(report.Details, "; "), false, report)
	}
	s.runner.setPolicy(active)
	s.runner.setDetection(engine)
	s.runner.setDetectionStatus(active, report, s.runner.contentStore().Snapshot())
	return detectionAck(s.runner.Config, req.GetContext(), active, report.Status, report.Message, false, report)
}

func (s *localControlServer) WatchEvents(req *controlplanev1.WatchEventsRequest, stream controlplanev1.AgentControlPlaneService_WatchEventsServer) error {
	if err := s.validateContext(req.GetContext()); err != nil {
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

func (s *localControlServer) WatchSignals(req *controlplanev1.WatchSignalsRequest, stream controlplanev1.AgentControlPlaneService_WatchSignalsServer) error {
	if err := s.validateContext(req.GetContext()); err != nil {
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

func (s *localControlServer) eventFrameByID(eventID string) (*controlplanev1.EventFrame, bool) {
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

func (s *localControlServer) recentEvents(req *controlplanev1.WatchEventsRequest) ([]*dataplanev1.EventFrame, error) {
	if s.runner.localStore == nil {
		return s.bus.SnapshotEvents(), nil
	}
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 100
	}
	return s.runner.localStore.QueryEvents(context.Background(), localstore.EventQuery{Behavior: req.GetBehavior(), AfterSequence: req.GetFilter().GetAfterSequence(), Limit: limit})
}

func (s *localControlServer) recentSignals(req *controlplanev1.WatchSignalsRequest) ([]*dataplanev1.SignalFrame, error) {
	if s.runner.localStore == nil {
		return s.bus.SnapshotSignals(), nil
	}
	limit := int(req.GetLimit())
	if limit == 0 {
		limit = 100
	}
	return s.runner.localStore.QuerySignals(context.Background(), localstore.SignalQuery{RuleID: req.GetRuleId(), Limit: limit})
}

func (r *AgentRuntime) localStoreHealth(ctx context.Context) (*controlplanev1.LocalStoreHealth, error) {
	stats, err := r.localStore.Stats(ctx)
	if err != nil {
		return nil, err
	}
	identity, err := r.localStore.DeviceIdentity(ctx)
	if err != nil {
		return nil, err
	}
	enrollment, err := r.localStore.Enrollment(ctx)
	if err != nil {
		return nil, err
	}
	checkpoint, err := r.localStore.Checkpoint(ctx)
	if err != nil {
		return nil, err
	}
	return &controlplanev1.LocalStoreHealth{Mode: string(enrollment.State), DeviceId: identity.DeviceID, StorageBytes: stats.StorageBytes,
		StorageMaxBytes: stats.StorageMaxBytes, OldestEventSequence: stats.OldestEventSequence, LatestEventSequence: stats.LatestEventSequence,
		SignalCount: stats.SignalCount, SealedSegmentCount: stats.SealedSegmentCount, OpenSegmentBytes: stats.OpenSegmentBytes,
		UploadSegmentId: checkpoint.SegmentID, UploadRecordOffset: checkpoint.RecordOffset, DroppedBatchesStorage: stats.DroppedBatchesStorage,
		DroppedEventsStorage: stats.DroppedEventsStorage}, nil
}

func (s *localControlServer) watchAfterBatchID(filter *controlplanev1.WatchFilter, includeRecent bool) string {
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

func (s *localControlServer) validateContext(ctx *controlplanev1.RequestContext) error {
	return s.runner.validateControlContext(ctx)
}

func (r *AgentRuntime) validateControlContext(ctx *controlplanev1.RequestContext) error {
	if ctx == nil {
		return nil
	}
	if tenantID := strings.TrimSpace(ctx.GetTenantId()); tenantID != "" && tenantID != r.Config.Agent.TenantID {
		return fmt.Errorf("tenant mismatch: request=%s agent=%s", tenantID, r.Config.Agent.TenantID)
	}
	if agentID := strings.TrimSpace(ctx.GetAgentId()); agentID != "" && agentID != r.Config.Agent.ID {
		return fmt.Errorf("agent mismatch: request=%s agent=%s", agentID, r.Config.Agent.ID)
	}
	return nil
}

func dataPlanePolicyFromRequest(req *controlplanev1.ApplyPolicyRequest, fallback *policymodel.DataPlanePolicy) (*policymodel.DataPlanePolicy, error) {
	if req.GetDataPlane() != nil {
		dataPlane := &policymodel.DataPlanePolicy{
			Transport:      req.GetDataPlane().GetTransport(),
			Endpoint:       req.GetDataPlane().GetEndpoint(),
			BatchSize:      int(req.GetDataPlane().GetBatchSize()),
			MaxBytes:       int(req.GetDataPlane().GetMaxBytes()),
			FlushInterval:  req.GetDataPlane().GetFlushInterval(),
			RetryInitial:   req.GetDataPlane().GetRetryInitial(),
			RetryMax:       req.GetDataPlane().GetRetryMax(),
			RequestTimeout: req.GetDataPlane().GetRequestTimeout(),
			MaxInflight:    int(req.GetDataPlane().GetMaxInflight()),
			Compression:    req.GetDataPlane().GetCompression(),
			TLSProfile:     req.GetDataPlane().GetTlsProfile(),
		}
		if err := validateDataPlanePolicy(dataPlane); err != nil {
			return nil, err
		}
		return dataPlane, nil
	}
	if fallback == nil {
		return nil, nil
	}
	dataPlane := *fallback
	if err := validateDataPlanePolicy(&dataPlane); err != nil {
		return nil, err
	}
	return &dataPlane, nil
}

func validateDataPlanePolicy(policy *policymodel.DataPlanePolicy) error {
	if policy == nil {
		return nil
	}
	switch strings.TrimSpace(policy.Transport) {
	case "", "grpc", "local":
	default:
		return fmt.Errorf("unsupported data_plane.transport %q", policy.Transport)
	}
	for name, value := range map[string]string{
		"flush_interval":  policy.FlushInterval,
		"retry_initial":   policy.RetryInitial,
		"retry_max":       policy.RetryMax,
		"request_timeout": policy.RequestTimeout,
	} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("data_plane.%s: %w", name, err)
		}
	}
	if policy.BatchSize < 0 {
		return fmt.Errorf("data_plane.batch_size must be non-negative")
	}
	if policy.MaxBytes < 0 {
		return fmt.Errorf("data_plane.max_bytes must be non-negative")
	}
	if policy.MaxInflight < 0 {
		return fmt.Errorf("data_plane.max_inflight must be non-negative")
	}
	switch strings.TrimSpace(policy.Compression) {
	case "", "none", "gzip", "zstd":
	default:
		return fmt.Errorf("unsupported data_plane.compression %q", policy.Compression)
	}
	if policy.RetryInitial != "" && policy.RetryMax != "" {
		initial, _ := time.ParseDuration(policy.RetryInitial)
		maximum, _ := time.ParseDuration(policy.RetryMax)
		if initial > maximum {
			return fmt.Errorf("data_plane.retry_initial must be <= data_plane.retry_max")
		}
	}
	return nil
}

func (r *AgentRuntime) applyDataPlaneConfig(dataPlane policymodel.DataPlanePolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if transport := strings.TrimSpace(dataPlane.Transport); transport != "" {
		r.Config.Manager.Transport = transport
	}
	if endpoint := strings.TrimSpace(dataPlane.Endpoint); endpoint != "" {
		r.Config.Manager.Address = endpoint
	}
	if dataPlane.BatchSize > 0 {
		r.Config.Telemetry.BatchSize = dataPlane.BatchSize
	}
	if dataPlane.MaxBytes > 0 {
		r.Config.Telemetry.MaxBytes = dataPlane.MaxBytes
	}
	if d := parseOptionalDuration(dataPlane.FlushInterval); d > 0 {
		r.Config.Telemetry.FlushInterval = d
	}
	if d := parseOptionalDuration(dataPlane.RetryInitial); d > 0 {
		r.Config.DataPlane.RetryInitial = d
	}
	if d := parseOptionalDuration(dataPlane.RetryMax); d > 0 {
		r.Config.DataPlane.RetryMax = d
	}
	if d := parseOptionalDuration(dataPlane.RequestTimeout); d > 0 {
		r.Config.DataPlane.RequestTimeout = d
	}
	if dataPlane.MaxInflight > 0 {
		r.Config.DataPlane.MaxInflight = dataPlane.MaxInflight
	}
	if compression := strings.TrimSpace(dataPlane.Compression); compression != "" {
		r.Config.DataPlane.Compression = compression
	}
	if tlsProfile := strings.TrimSpace(dataPlane.TLSProfile); tlsProfile != "" {
		r.Config.DataPlane.TLSProfile = tlsProfile
	}
}

func parseOptionalDuration(value string) time.Duration {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	d, _ := time.ParseDuration(value)
	return d
}

func rejectedAck(cfg config.Config, req *controlplanev1.RequestContext, section, message string) *controlplanev1.ControlAck {
	return &controlplanev1.ControlAck{
		RequestId: requestID(req),
		TenantId:  cfg.Agent.TenantID,
		AgentId:   cfg.Agent.ID,
		Status:    "rejected",
		Message:   message,
		Sections: []*controlplanev1.AppliedSection{{
			Name:    section,
			Status:  "rejected",
			Message: message,
		}},
	}
}

func dataPlaneAck(cfg config.Config, req *controlplanev1.RequestContext, status, message string, requiresRestart bool, dataPlane policymodel.DataPlanePolicy) *controlplanev1.ControlAck {
	report, _ := json.Marshal(map[string]any{"data_plane": dataPlane})
	return &controlplanev1.ControlAck{
		RequestId: requestID(req),
		TenantId:  cfg.Agent.TenantID,
		AgentId:   cfg.Agent.ID,
		Status:    status,
		Message:   message,
		Sections: []*controlplanev1.AppliedSection{{
			Name:            "data_plane",
			Status:          status,
			Message:         message,
			RequiresRestart: requiresRestart,
			ReportJson:      string(report),
		}},
		ReportJson: string(report),
	}
}

func appliedAck(cfg config.Config, req *controlplanev1.RequestContext, policy policymodel.Policy, status, message string, requiresRestart bool) *controlplanev1.ControlAck {
	dataPlaneStatus := "unchanged"
	dataPlaneMessage := "data plane policy unchanged"
	dataPlaneRequiresRestart := requiresRestart
	if policy.DataPlane != nil {
		dataPlaneStatus = status
		dataPlaneMessage = "data plane policy accepted; restart data batch dispatcher to take effect"
		dataPlaneRequiresRestart = true
	}
	return &controlplanev1.ControlAck{
		RequestId:     requestID(req),
		TenantId:      cfg.Agent.TenantID,
		AgentId:       cfg.Agent.ID,
		Status:        status,
		Message:       message,
		PolicyId:      policy.PolicyID,
		PolicyVersion: policy.Version,
		Sections: []*controlplanev1.AppliedSection{
			{Name: "detection", Status: status, Message: "endpoint rules updated", RequiresRestart: false},
			{Name: "response", Status: status, Message: "response policy updated", RequiresRestart: false},
			{Name: "resource", Status: "unsupported", Message: "resource policy contract is reserved for the next phase", RequiresRestart: requiresRestart},
			{Name: "data_plane", Status: dataPlaneStatus, Message: dataPlaneMessage, RequiresRestart: dataPlaneRequiresRestart},
			{Name: "collection", Status: "unsupported", Message: "collection hot reload requires compiler/runtime apply in the next phase", RequiresRestart: true},
		},
	}
}

func requestID(req *controlplanev1.RequestContext) string {
	if req == nil {
		return ""
	}
	return req.GetRequestId()
}

func requestScope(req *controlplanev1.RequestContext) config.RuntimeScope {
	if req == nil || req.GetScope() == nil {
		return config.RuntimeScope{}
	}
	return config.RuntimeScope{Type: req.GetScope().GetType(), Selector: req.GetScope().GetSelector()}
}

func collectionContentSnapshot(snapshot agentcontent.Snapshot) agentpolicy.CollectionContentSnapshot {
	out := agentpolicy.CollectionContentSnapshot{
		ContextSets: make(map[string]agentpolicy.CollectionValueSet, len(snapshot.ContextSets)),
		IOCPacks:    make(map[string]agentpolicy.CollectionValueSet, len(snapshot.IOCPacks)),
	}
	for ref, set := range snapshot.ContextSets {
		out.ContextSets[ref] = collectionValueSet(set)
	}
	for ref, set := range snapshot.IOCPacks {
		out.IOCPacks[ref] = collectionValueSet(set)
	}
	return out
}

func collectionValueSet(set agentcontent.ValueSet) agentpolicy.CollectionValueSet {
	return agentpolicy.CollectionValueSet{
		Ref:       set.Ref,
		Version:   set.Version,
		Digest:    set.Digest,
		ValueType: set.ValueType,
		Values:    append([]string(nil), set.Values...),
	}
}

type collectionExplainReport struct {
	contract.CollectionCompileReport
	DetectionCoverage *detection.CoverageReport `json:"detection_coverage,omitempty"`
}

func collectionAck(cfg config.Config, req *controlplanev1.RequestContext, policy agentpolicy.CollectionPolicy, status, message string, requiresRestart bool, report contract.CollectionCompileReport, coverage *detection.CoverageReport) *controlplanev1.ControlAck {
	details := collectionReportDetails(report, coverage)
	reportJSON := collectionReportJSON(report, coverage)
	return &controlplanev1.ControlAck{
		RequestId:     requestID(req),
		TenantId:      cfg.Agent.TenantID,
		AgentId:       cfg.Agent.ID,
		Status:        status,
		Message:       message,
		PolicyId:      policy.PolicyID,
		PolicyVersion: policy.Version,
		Details:       details,
		ReportJson:    reportJSON,
		Sections: []*controlplanev1.AppliedSection{{
			Name:            "collection",
			Status:          status,
			Message:         message,
			RequiresRestart: requiresRestart,
			Details:         details,
			ReportJson:      reportJSON,
		}},
	}
}

func collectionReportDetails(report contract.CollectionCompileReport, coverage *detection.CoverageReport) []string {
	if report.Backend == "" {
		return nil
	}
	details := []string{
		fmt.Sprintf("backend=%s", report.Backend),
		fmt.Sprintf("pushed_down_selectors=%d", len(report.PushedDownSelectors)),
		fmt.Sprintf("agent_side_selectors=%d", len(report.AgentSideSelectors)),
		fmt.Sprintf("unsupported_selectors=%d", len(report.UnsupportedSelectors)),
	}
	if len(report.ResolvedRefs) > 0 {
		details = append(details, fmt.Sprintf("resolved_refs=%d", len(report.ResolvedRefs)))
	}
	if report.GeneratedPolicyHash != "" {
		details = append(details, "generated_policy_hash="+report.GeneratedPolicyHash)
	}
	for _, warning := range report.Warnings {
		if warning != "" {
			details = append(details, "warning="+warning)
		}
	}
	if coverage != nil && coverage.Status != "" {
		details = append(details, "detection_coverage="+coverage.Status)
		for _, warning := range coverage.Warnings {
			if warning != "" {
				details = append(details, "coverage_warning="+warning)
			}
		}
	}
	return details
}

func collectionReportJSON(report contract.CollectionCompileReport, coverage *detection.CoverageReport) string {
	if report.Backend == "" {
		return ""
	}
	out := collectionExplainReport{CollectionCompileReport: report}
	if coverage != nil {
		out.DetectionCoverage = coverage
	}
	data, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(data)
}

func detectionAck(cfg config.Config, req *controlplanev1.RequestContext, policy policymodel.Policy, status, message string, requiresRestart bool, report detection.ApplyReport) *controlplanev1.ControlAck {
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
	reportJSON := detectionReportJSON(report)
	return &controlplanev1.ControlAck{
		RequestId:     requestID(req),
		TenantId:      cfg.Agent.TenantID,
		AgentId:       cfg.Agent.ID,
		Status:        status,
		Message:       sectionMessage,
		PolicyId:      policy.PolicyID,
		PolicyVersion: policy.Version,
		Sections: []*controlplanev1.AppliedSection{{
			Name:            "detection",
			Status:          status,
			Message:         sectionMessage,
			RequiresRestart: requiresRestart,
			ReportJson:      reportJSON,
		}},
		ReportJson: reportJSON,
	}
}

func detectionReportJSON(report detection.ApplyReport) string {
	data, err := json.Marshal(report)
	if err != nil {
		return ""
	}
	return string(data)
}

func contentRecordMessage(record agentcontent.Record) *controlplanev1.ContentRecord {
	return &controlplanev1.ContentRecord{
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

func healthResponse(health agenthealth.AgentHealth) *controlplanev1.HealthResponse {
	return &controlplanev1.HealthResponse{
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
		Sensor: &controlplanev1.SensorHealth{
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
		TelemetryBus: &controlplanev1.TelemetryBusHealth{
			EventCapacity:     health.TelemetryBus.EventCapacity,
			EventBuffered:     health.TelemetryBus.EventBuffered,
			EventDropped:      health.TelemetryBus.EventDropped,
			EventSubscribers:  health.TelemetryBus.EventSubscribers,
			SignalCapacity:    health.TelemetryBus.SignalCapacity,
			SignalBuffered:    health.TelemetryBus.SignalBuffered,
			SignalDropped:     health.TelemetryBus.SignalDropped,
			SignalSubscribers: health.TelemetryBus.SignalSubscribers,
		},
		TelemetryBatcher: &controlplanev1.TelemetryBatcherHealth{
			PendingEvents:     health.TelemetryBatcher.PendingEvents,
			PendingSignals:    health.TelemetryBatcher.PendingSignals,
			QueuedBatches:     health.TelemetryBatcher.QueuedBatches,
			QueueCapacity:     health.TelemetryBatcher.QueueCapacity,
			DroppedBatches:    health.TelemetryBatcher.DroppedBatches,
			DroppedEvents:     health.TelemetryBatcher.DroppedEvents,
			DroppedSignals:    health.TelemetryBatcher.DroppedSignals,
			FlushedBatches:    health.TelemetryBatcher.FlushedBatches,
			FlushedEvents:     health.TelemetryBatcher.FlushedEvents,
			FlushedSignals:    health.TelemetryBatcher.FlushedSignals,
			PendingBytes:      health.TelemetryBatcher.PendingBytes,
			MaxBytes:          health.TelemetryBatcher.MaxBytes,
			FlushedByCount:    health.TelemetryBatcher.FlushedByCount,
			FlushedByBytes:    health.TelemetryBatcher.FlushedByBytes,
			FlushedByInterval: health.TelemetryBatcher.FlushedByInterval,
			FlushedByShutdown: health.TelemetryBatcher.FlushedByShutdown,
			LastFlushReason:   health.TelemetryBatcher.LastFlushReason,
			Closed:            health.TelemetryBatcher.Closed,
			LastError:         health.TelemetryBatcher.LastError,
		},
		TelemetrySender: &controlplanev1.TelemetrySenderHealth{
			SentBatches:     health.TelemetrySender.SentBatches,
			SentEvents:      health.TelemetrySender.SentEvents,
			SentSignals:     health.TelemetrySender.SentSignals,
			RejectedBatches: health.TelemetrySender.RejectedBatches,
			RetriedBatches:  health.TelemetrySender.RetriedBatches,
			Drained:         health.TelemetrySender.Drained,
			LastError:       health.TelemetrySender.LastError,
		},
		Detection: &controlplanev1.DetectionRuntimeHealth{
			PolicyId:        health.Detection.PolicyID,
			PolicyVersion:   health.Detection.PolicyVersion,
			ContentRefs:     detectionContentRefMessages(health.Detection.ContentRefs),
			LastApplyStatus: health.Detection.LastApplyStatus,
			LastApplyError:  health.Detection.LastApplyError,
			UpdatedAt:       timestampString(health.Detection.UpdatedAt),
		},
		Cep: &controlplanev1.CEPHealth{
			ActiveGroups:     health.CEP.ActiveGroups,
			EvictedGroups:    health.CEP.EvictedGroups,
			ExpiredGroups:    health.CEP.ExpiredGroups,
			DroppedEventRefs: health.CEP.DroppedEventRefs,
			EvalErrors:       health.CEP.EvalErrors,
			EmittedSignals:   health.CEP.EmittedSignals,
			Degraded:         health.CEP.Degraded,
		},
		Streams: &controlplanev1.LocalStreamHealth{
			EventCapacity:        health.Streams.EventCapacity,
			EventBuffered:        health.Streams.EventBuffered,
			EventNextSequence:    health.Streams.EventNextSequence,
			EventOldestSequence:  health.Streams.EventOldestSequence,
			EventNewestSequence:  health.Streams.EventNewestSequence,
			EventEvicted:         health.Streams.EventEvicted,
			EventSubscribers:     health.Streams.EventSubscribers,
			SignalCapacity:       health.Streams.SignalCapacity,
			SignalBuffered:       health.Streams.SignalBuffered,
			SignalNextSequence:   health.Streams.SignalNextSequence,
			SignalOldestSequence: health.Streams.SignalOldestSequence,
			SignalNewestSequence: health.Streams.SignalNewestSequence,
			SignalEvicted:        health.Streams.SignalEvicted,
			SignalSubscribers:    health.Streams.SignalSubscribers,
		},
		ObservedAt: timestampString(health.ObservedAt),
	}
}

func detectionContentRefMessages(in []agenthealth.ContentRef) []*controlplanev1.DetectionContentRef {
	out := make([]*controlplanev1.DetectionContentRef, 0, len(in))
	for _, item := range in {
		out = append(out, &controlplanev1.DetectionContentRef{
			Ref:     item.Ref,
			Kind:    item.Kind,
			Version: item.Version,
			Digest:  item.Digest,
		})
	}
	return out
}

func scopeMessage(scope agenthealth.RuntimeScope) *controlplanev1.Scope {
	return &controlplanev1.Scope{Type: scope.Type, Selector: scope.Selector}
}

func capabilityMessage(cap agenthealth.SensorCapability) *controlplanev1.SensorCapability {
	return &controlplanev1.SensorCapability{
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

func collectionBehaviorMessages(in []agenthealth.CollectionBehaviorCapability) []*controlplanev1.CollectionBehaviorCapability {
	out := make([]*controlplanev1.CollectionBehaviorCapability, 0, len(in))
	for _, item := range in {
		out = append(out, &controlplanev1.CollectionBehaviorCapability{
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
