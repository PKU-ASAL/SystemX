package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/localstore"
	agenthealth "github.com/sysarmor/sysarmor-next-project/packages/contracts/health"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	policymodel "github.com/sysarmor/sysarmor-next-project/packages/policy"
)

func (s *localControlServer) Health(ctx context.Context, req *controlplanev1.HealthRequest) (*controlplanev1.HealthResponse, error) {
	health, err := s.runner.collectHealth(ctx, s.runtime, s.bus, s.batcher, s.sender, s.startedAt)
	if err != nil {
		return nil, err
	}
	response := healthResponse(health)
	if s.runner.localStore != nil {
		if response.LocalStore, err = s.runner.localStoreHealth(ctx); err != nil {
			return nil, err
		}
		if response.ManagementLifecycle, err = s.runner.managementLifecycleStatus(ctx); err != nil {
			return nil, err
		}
		if managementLifecycleDegraded(response.ManagementLifecycle) {
			response.Status = "degraded"
		}
	}
	return response, nil
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
			"telemetry",
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
	document := any(policy)
	if endpoint := s.runner.currentEndpointPolicy(); endpoint.PolicyID != "" {
		document = endpoint
		policy.PolicyID = endpoint.PolicyID
		policy.Version = endpoint.Version
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	pending, err := s.runner.pendingPolicyStatus(ctx)
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
		Mode:          policy.Mode,
		CloudRules:    append([]string(nil), policy.CloudRules...),
		Published:     policy.Published,
		RawJson:       string(raw),
		PendingPolicy: pendingPolicyMessage(pending),
	}, nil
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

func (r *AgentRuntime) managementLifecycleStatus(ctx context.Context) (*controlplanev1.ManagementLifecycleStatus, error) {
	enrollment, err := r.localStore.Enrollment(ctx)
	if err != nil {
		return nil, err
	}
	status := &controlplanev1.ManagementLifecycleStatus{
		Mode:                string(enrollment.State),
		TransitionPhase:     enrollment.TransitionPhase,
		RevocationConfirmed: enrollment.RevocationConfirmed,
		LastTransitionError: enrollment.LastTransitionError,
		UpdatedAt:           timestampString(enrollment.UpdatedAt),
	}
	completion, ok, err := r.localStore.UnenrollmentCompletion(ctx)
	if err != nil {
		return nil, err
	}
	if ok {
		if completion.Status == localstore.CompletionPrepared {
			status.ManagerCompletionStatus = "revocation_pending"
		} else if completion.Status == localstore.CompletionReady {
			status.ManagerCompletionStatus = "endpoint_completion_pending"
		}
		status.UpdatedAt = timestampString(completion.UpdatedAt)
		if completion.LastError != "" {
			status.LastTransitionError = completion.LastError
		}
	}
	return status, nil
}

func managementLifecycleDegraded(status *controlplanev1.ManagementLifecycleStatus) bool {
	if status == nil {
		return false
	}
	return status.GetTransitionPhase() != "" || status.GetManagerCompletionStatus() != "" || status.GetLastTransitionError() != ""
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
		PendingPolicy: pendingPolicyMessage(health.PendingPolicy),
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
			PolicyId:               health.Detection.PolicyID,
			PolicyVersion:          health.Detection.PolicyVersion,
			ContentRefs:            detectionContentRefMessages(health.Detection.ContentRefs),
			LastApplyStatus:        health.Detection.LastApplyStatus,
			LastApplyError:         health.Detection.LastApplyError,
			UpdatedAt:              timestampString(health.Detection.UpdatedAt),
			DefaultManifestVersion: health.Detection.DefaultManifestVersion,
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
