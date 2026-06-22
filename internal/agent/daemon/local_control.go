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
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/tetragon"
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
	if policyType == "upload" {
		return s.applyUploadPolicy(req, nil), nil
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
	uploadSection, err := uploadPolicyFromRequest(req, next.Upload)
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "upload", err.Error()), nil
	}
	if uploadSection != nil {
		next.Upload = uploadSection
	}
	if req.GetDryRun() {
		return appliedAck(s.runner.Config, req.GetContext(), next, "validated", "policy accepted in dry-run", uploadSection != nil), nil
	}
	s.runner.applyRuntimePolicy(next)
	if uploadSection != nil {
		s.runner.applyUploadConfig(*uploadSection)
	}
	return appliedAck(s.runner.Config, req.GetContext(), next, "applied", "runtime policy applied", uploadSection != nil), nil
}

func (s *localControlServer) applyUploadPolicy(req *controlv1.ApplyPolicyRequest, fallback *policymodel.UploadPolicy) *controlv1.ControlAck {
	if fallback == nil && strings.TrimSpace(req.GetPolicyJson()) != "" {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(req.GetPolicyJson()), &raw); err != nil {
			return rejectedAck(s.runner.Config, req.GetContext(), "upload", "invalid upload policy json: "+err.Error())
		}
		payload := []byte(req.GetPolicyJson())
		if nested, ok := raw["upload"]; ok {
			payload = nested
		}
		var upload policymodel.UploadPolicy
		if err := json.Unmarshal(payload, &upload); err != nil {
			return rejectedAck(s.runner.Config, req.GetContext(), "upload", "invalid upload policy json: "+err.Error())
		}
		fallback = &upload
	}
	upload, err := uploadPolicyFromRequest(req, fallback)
	if err != nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "upload", err.Error())
	}
	if upload == nil {
		return rejectedAck(s.runner.Config, req.GetContext(), "upload", "upload policy is required")
	}
	if req.GetDryRun() {
		return uploadAck(s.runner.Config, req.GetContext(), "validated", "upload policy accepted in dry-run", true, *upload)
	}
	s.runner.applyUploadConfig(*upload)
	return uploadAck(s.runner.Config, req.GetContext(), "applied", "upload policy applied; restart upload worker to take effect", true, *upload)
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
	frame, ok := s.eventFrameByID(eventID)
	if !ok {
		return nil, fmt.Errorf("event %q not found in spool WAL", eventID)
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
		for _, entry := range s.snapshotEntries(req.GetFilter(), req.GetIncludeRecent()) {
			if err := s.sendEventEntry(entry.ID, send); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
		return nil
	}
	entries, err := s.queue.Watch(stream.Context(), s.watchAfterBatchID(req.GetFilter(), req.GetIncludeRecent()))
	if err != nil {
		return err
	}
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case entry, ok := <-entries:
			if !ok {
				return nil
			}
			if err := s.sendEventEntry(entry.ID, send); err != nil {
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
		for _, entry := range s.snapshotEntries(req.GetFilter(), req.GetIncludeRecent()) {
			if err := s.sendSignalEntry(entry.ID, send); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
		return nil
	}
	entries, err := s.queue.Watch(stream.Context(), s.watchAfterBatchID(req.GetFilter(), req.GetIncludeRecent()))
	if err != nil {
		return err
	}
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case entry, ok := <-entries:
			if !ok {
				return nil
			}
			if err := s.sendSignalEntry(entry.ID, send); err != nil {
				return err
			}
			if req.GetLimit() > 0 && sent >= req.GetLimit() {
				return nil
			}
		}
	}
}

func (s *localControlServer) snapshotEntries(filter *controlv1.WatchFilter, includeRecent bool) []spool.Entry {
	if !includeRecent {
		return nil
	}
	entries, err := s.queue.SnapshotAfter(s.watchAfterBatchID(filter, true))
	if err != nil {
		return nil
	}
	return entries
}

func (s *localControlServer) sendEventEntry(id string, send func(*controlv1.EventFrame) error) error {
	batch, err := s.queue.LoadDataBatch(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	header := batch.GetHeader()
	for _, frame := range batch.GetEvents() {
		out := &controlv1.EventFrame{
			TenantId:   header.GetTenantId(),
			AgentId:    header.GetAgentId(),
			Sequence:   frame.GetSequence(),
			ObservedAt: frame.GetObservedAt(),
			Event:      frame.GetEvent(),
		}
		if err := send(out); err != nil {
			return err
		}
	}
	return nil
}

func (s *localControlServer) sendSignalEntry(id string, send func(*controlv1.SignalFrame) error) error {
	batch, err := s.queue.LoadDataBatch(id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	header := batch.GetHeader()
	for _, frame := range batch.GetSignals() {
		out := &controlv1.SignalFrame{
			TenantId:   header.GetTenantId(),
			AgentId:    header.GetAgentId(),
			Sequence:   frame.GetSequence(),
			ObservedAt: frame.GetObservedAt(),
			Signal:     frame.GetSignal(),
		}
		if err := send(out); err != nil {
			return err
		}
	}
	return nil
}

func (s *localControlServer) eventFrameByID(eventID string) (*controlv1.EventFrame, bool) {
	entries, err := s.queue.SnapshotAfter("")
	if err != nil {
		return nil, false
	}
	for _, entry := range entries {
		var found *controlv1.EventFrame
		err := s.sendEventEntry(entry.ID, func(frame *controlv1.EventFrame) error {
			if frame.GetEvent().GetId() == eventID {
				found = frame
			}
			return nil
		})
		if err != nil {
			continue
		}
		if found != nil {
			return found, true
		}
	}
	return nil, false
}

func (s *localControlServer) watchAfterBatchID(filter *controlv1.WatchFilter, includeRecent bool) string {
	if filter != nil && strings.TrimSpace(filter.GetAfterBatchId()) != "" {
		return strings.TrimSpace(filter.GetAfterBatchId())
	}
	if includeRecent {
		return ""
	}
	entries, err := s.queue.SnapshotAfter("")
	if err != nil || len(entries) == 0 {
		return ""
	}
	return entries[len(entries)-1].ID
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

func uploadPolicyFromRequest(req *controlv1.ApplyPolicyRequest, fallback *policymodel.UploadPolicy) (*policymodel.UploadPolicy, error) {
	if req.GetUpload() != nil {
		upload := &policymodel.UploadPolicy{
			Transport:      req.GetUpload().GetTransport(),
			Endpoint:       req.GetUpload().GetEndpoint(),
			BatchSize:      int(req.GetUpload().GetBatchSize()),
			FlushInterval:  req.GetUpload().GetFlushInterval(),
			RetryInitial:   req.GetUpload().GetRetryInitial(),
			RetryMax:       req.GetUpload().GetRetryMax(),
			RequestTimeout: req.GetUpload().GetRequestTimeout(),
			MaxInflight:    int(req.GetUpload().GetMaxInflight()),
			Compression:    req.GetUpload().GetCompression(),
			TLSProfile:     req.GetUpload().GetTlsProfile(),
		}
		if err := validateUploadPolicy(upload); err != nil {
			return nil, err
		}
		return upload, nil
	}
	if fallback == nil {
		return nil, nil
	}
	upload := *fallback
	if err := validateUploadPolicy(&upload); err != nil {
		return nil, err
	}
	return &upload, nil
}

func validateUploadPolicy(upload *policymodel.UploadPolicy) error {
	if upload == nil {
		return nil
	}
	switch strings.TrimSpace(upload.Transport) {
	case "", "grpc", "local":
	default:
		return fmt.Errorf("unsupported upload.transport %q", upload.Transport)
	}
	for name, value := range map[string]string{
		"flush_interval":  upload.FlushInterval,
		"retry_initial":   upload.RetryInitial,
		"retry_max":       upload.RetryMax,
		"request_timeout": upload.RequestTimeout,
	} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("upload.%s: %w", name, err)
		}
	}
	if upload.BatchSize < 0 {
		return fmt.Errorf("upload.batch_size must be non-negative")
	}
	if upload.MaxInflight < 0 {
		return fmt.Errorf("upload.max_inflight must be non-negative")
	}
	switch strings.TrimSpace(upload.Compression) {
	case "", "none", "gzip", "zstd":
	default:
		return fmt.Errorf("unsupported upload.compression %q", upload.Compression)
	}
	if upload.RetryInitial != "" && upload.RetryMax != "" {
		initial, _ := time.ParseDuration(upload.RetryInitial)
		maximum, _ := time.ParseDuration(upload.RetryMax)
		if initial > maximum {
			return fmt.Errorf("upload.retry_initial must be <= upload.retry_max")
		}
	}
	return nil
}

func (r *Runner) applyUploadConfig(upload policymodel.UploadPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if transport := strings.TrimSpace(upload.Transport); transport != "" {
		r.Config.Manager.Transport = transport
	}
	if endpoint := strings.TrimSpace(upload.Endpoint); endpoint != "" {
		r.Config.Manager.Address = endpoint
	}
	if upload.BatchSize > 0 {
		r.Config.Spool.BatchSize = upload.BatchSize
	}
	if d := parseOptionalDuration(upload.FlushInterval); d > 0 {
		r.Config.Spool.FlushInterval = d
	}
	if d := parseOptionalDuration(upload.RetryInitial); d > 0 {
		r.Config.Upload.RetryInitial = d
	}
	if d := parseOptionalDuration(upload.RetryMax); d > 0 {
		r.Config.Upload.RetryMax = d
	}
	if d := parseOptionalDuration(upload.RequestTimeout); d > 0 {
		r.Config.Upload.RequestTimeout = d
	}
	if upload.MaxInflight > 0 {
		r.Config.Upload.MaxInflight = upload.MaxInflight
	}
	if compression := strings.TrimSpace(upload.Compression); compression != "" {
		r.Config.Upload.Compression = compression
	}
	if tlsProfile := strings.TrimSpace(upload.TLSProfile); tlsProfile != "" {
		r.Config.Upload.TLSProfile = tlsProfile
	}
}

func parseOptionalDuration(value string) time.Duration {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	d, _ := time.ParseDuration(value)
	return d
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

func uploadAck(cfg config.Config, req *controlv1.RequestContext, status, message string, requiresRestart bool, upload policymodel.UploadPolicy) *controlv1.ControlAck {
	report, _ := json.Marshal(map[string]any{"upload": upload})
	return &controlv1.ControlAck{
		RequestId: requestID(req),
		TenantId:  cfg.Agent.TenantID,
		AgentId:   cfg.Agent.ID,
		Status:    status,
		Message:   message,
		Sections: []*controlv1.AppliedSection{{
			Name:            "upload",
			Status:          status,
			Message:         message,
			RequiresRestart: requiresRestart,
			ReportJson:      string(report),
		}},
		ReportJson: string(report),
	}
}

func appliedAck(cfg config.Config, req *controlv1.RequestContext, policy policymodel.Policy, status, message string, requiresRestart bool) *controlv1.ControlAck {
	uploadStatus := "unchanged"
	uploadMessage := "upload policy unchanged"
	uploadRequiresRestart := requiresRestart
	if policy.Upload != nil {
		uploadStatus = status
		uploadMessage = "upload policy accepted; restart upload worker to take effect"
		uploadRequiresRestart = true
	}
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
			{Name: "upload", Status: uploadStatus, Message: uploadMessage, RequiresRestart: uploadRequiresRestart},
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

func collectionAck(cfg config.Config, req *controlv1.RequestContext, policy agentpolicy.CollectionPolicy, status, message string, requiresRestart bool, report contract.CollectionCompileReport, coverage *detection.CoverageReport) *controlv1.ControlAck {
	details := collectionReportDetails(report, coverage)
	reportJSON := collectionReportJSON(report, coverage)
	return &controlv1.ControlAck{
		RequestId:     requestID(req),
		TenantId:      cfg.Agent.TenantID,
		AgentId:       cfg.Agent.ID,
		Status:        status,
		Message:       message,
		PolicyId:      policy.PolicyID,
		PolicyVersion: policy.Version,
		Details:       details,
		ReportJson:    reportJSON,
		Sections: []*controlv1.AppliedSection{{
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
	reportJSON := detectionReportJSON(report)
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

func eventFrameMatches(frame *controlv1.EventFrame, behavior string, filter *controlv1.WatchFilter) bool {
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

func signalFrameMatches(frame *controlv1.SignalFrame, ruleID, where string, filter *controlv1.WatchFilter) bool {
	if frame == nil || !signalMatches(frame.GetSignal(), ruleID, where) {
		return false
	}
	return frameMatches(frame.GetSequence(), frame.GetObservedAt(), frame.GetSignal().GetLabels(), filter)
}

func frameMatches(sequence uint64, observedAt string, labels map[string]string, filter *controlv1.WatchFilter) bool {
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
		Wal: &controlv1.WALHealth{
			QueuedBatches:     uint32(health.WAL.QueuedBatches),
			QueuedBytes:       health.WAL.QueuedBytes,
			MaxBytes:          health.WAL.MaxBytes,
			OldestBatchId:     health.WAL.OldestBatchID,
			NewestBatchId:     health.WAL.NewestBatchID,
			LastAckedBatchId:  health.WAL.LastAckedBatchID,
			WatchSubscribers:  health.WAL.WatchSubscribers,
			BackpressureCount: health.WAL.BackpressureCount,
			DroppedBatches:    health.WAL.DroppedBatches,
			DroppedBytes:      health.WAL.DroppedBytes,
			LastError:         health.WAL.LastError,
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
		Streams: &controlv1.LocalStreamHealth{
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
