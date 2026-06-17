package daemon

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agentcontent "github.com/sysarmor/sysarmor-next-project/internal/agent/content"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/tamper"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/uploadworker"
	gatewaymodel "github.com/sysarmor/sysarmor-next-project/internal/agentgateway/model"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/detection"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/uploader"
	"github.com/sysarmor/sysarmor-next-project/internal/eventmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/fake"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/tetragon"
	"google.golang.org/protobuf/encoding/protojson"
)

type localHealthReporter struct{}

func (localHealthReporter) Report(context.Context, agenthealth.AgentHealth) error {
	return nil
}

type localBatchUploader struct{}

func (localBatchUploader) Upload(batch *analyticsv1.UploadBatch) (*analyticsv1.UploadAck, error) {
	if batch == nil {
		return &analyticsv1.UploadAck{Ok: true}, nil
	}
	return &analyticsv1.UploadAck{
		Ok:              true,
		BatchId:         batch.GetBatchId(),
		AcceptedEvents:  uint64(len(batch.GetEvents())),
		AcceptedSignals: uint64(len(batch.GetSignals())),
	}, nil
}

type Options struct {
	Once      bool
	DrainOnce bool
	Out       io.Writer
}

type Runner struct {
	Config     config.Config
	Sensor     contract.Sensor
	Out        io.Writer
	capability contract.Capability
	mu         sync.RWMutex
	policy     policymodel.Policy
	detection  *detection.Engine
	collection contract.CollectionIntent
	streams    *localStreamBuffer
	content    *agentcontent.Store
}

type healthReporter interface {
	Report(context.Context, agenthealth.AgentHealth) error
}

func New(cfg config.Config) (*Runner, error) {
	sensor, err := sensorFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	contentStore, err := agentcontent.NewStoreWithOptions(agentcontent.Options{Dir: cfg.Content.Path, TrustedKeys: parseTrustKeys(cfg.Content.TrustKeys)})
	if err != nil {
		return nil, err
	}
	return &Runner{Config: cfg, Sensor: sensor, streams: newLocalStreamBuffer(defaultLocalStreamCapacity), content: contentStore}, nil
}

func (r *Runner) Run(ctx context.Context, opts Options) error {
	if opts.Out != nil {
		r.Out = opts.Out
	}
	startedAt := time.Now()
	reporter := r.healthReporter()
	failStartup := func(stage string, err error) error {
		r.reportStartupFailure(reporter, startedAt, stage, err)
		return err
	}
	rt := sensorruntime.New(r.Sensor)
	capability, err := rt.Probe(ctx)
	if err != nil {
		return failStartup("probe", err)
	}
	r.capability = capability
	intent, err := policy.LoadCollectionIntent(r.Config.Sensor.PolicyPath, r.Config.Sensor.ObserveOnly)
	if err != nil {
		return failStartup("policy", err)
	}
	scope, err := r.Config.Sensor.EffectiveScope()
	if err != nil {
		return err
	}
	scopeType := scope.Type
	scopeSelector := scope.Selector
	intent = policy.WithScope(intent, scopeType, scopeSelector)
	r.setCollectionIntent(r.withCollectionCapabilities(intent))
	if err := rt.Apply(ctx, intent); err != nil {
		return failStartup("apply", err)
	}
	effectivePolicy := r.fetchStartupPolicy(ctx, scopeType, scopeSelector)
	events, err := rt.Subscribe(ctx)
	if err != nil {
		return failStartup("subscribe", err)
	}
	queue, err := spool.OpenWithLimit(r.Config.Spool.Path, r.Config.Spool.MaxBytes)
	if err != nil {
		return failStartup("spool", err)
	}
	worker, err := r.uploadWorker(queue)
	if err != nil {
		return failStartup("upload", err)
	}
	stopLocalControl := func() {}
	if !opts.Once && !opts.DrainOnce {
		stopLocalControl, err = r.startLocalControlServer(ctx, rt, queue, worker, startedAt)
		if err != nil {
			return failStartup("local_control", err)
		}
		defer stopLocalControl()
	}
	if r.Config.Manager.Transport == "http" || r.Config.Manager.Transport == "stream" {
		stats, err := worker.ResumeOnce(ctx)
		if err != nil {
			return failStartup("resume", err)
		}
		if r.Out != nil && stats.LastError != "" {
			fmt.Fprintf(r.Out, "agent agentgateway resume error: %s\n", stats.LastError)
		}
	}
	var responseClient *ResponseClient
	if r.Config.Manager.Transport == "http" {
		responseClient = NewResponseClient(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout)
	}
	var streamResponseClient *StreamResponseClient
	var streamEvidenceClient *StreamEvidenceClient
	if r.Config.Manager.Transport == "stream" {
		streamResponseClient = NewStreamResponseClient(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout)
		streamEvidenceClient = NewStreamEvidenceClient(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout)
	}
	uploadCtx := ctx
	cancelUploads := func() {}
	if !opts.Once && !opts.DrainOnce {
		uploadCtx, cancelUploads = context.WithCancel(ctx)
		defer cancelUploads()
		go runUploadLoop(uploadCtx, worker, r.Config.Spool.FlushInterval)
	}
	norm := normalize.NewWithOptions(r.Config.Agent.ID, r.Config.Agent.HostID, nil, normalize.Options{
		TenantID:      r.Config.Agent.TenantID,
		ScopeType:     scopeType,
		ScopeSelector: scopeSelector,
	})
	r.applyRuntimePolicy(effectivePolicy)
	refreshCtx := ctx
	cancelRefresh := func() {}
	if !opts.Once && r.Config.Policy.RefreshInterval > 0 && (r.Config.Manager.Transport == "http" || r.Config.Manager.Transport == "stream") {
		refreshCtx, cancelRefresh = context.WithCancel(ctx)
		defer cancelRefresh()
		go r.runPolicyRefreshLoop(refreshCtx, scopeType, scopeSelector)
	}
	if r.Out != nil {
		fmt.Fprintf(r.Out, "agent daemon started: agent=%s host=%s tenant=%s sensor=%s version=%s behaviors=%d policy=%s version=%d mode=%s\n",
			r.Config.Agent.ID, r.Config.Agent.HostID, r.Config.Agent.TenantID, capability.Backend, capability.Version, len(intent.Behaviors), effectivePolicy.PolicyID, effectivePolicy.Version, effectivePolicy.Mode)
	}

	ticker := time.NewTicker(r.Config.Health.Interval)
	defer ticker.Stop()
	tamperDetector := &tamper.Detector{}
	stopped := false
	stopRuntime := func() {
		if stopped {
			return
		}
		stopped = true
		_ = rt.Stop(context.Background())
	}
	defer stopRuntime()

	for {
		select {
		case <-ctx.Done():
			drainErr := r.shutdownAndReport(context.Background(), rt, queue, worker, reporter, startedAt, opts.DrainOnce, cancelUploads, stopRuntime)
			if drainErr != nil && !errors.Is(drainErr, context.DeadlineExceeded) && !errors.Is(drainErr, context.Canceled) {
				return drainErr
			}
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				drainErr := r.shutdownAndReport(context.Background(), rt, queue, worker, reporter, startedAt, opts.DrainOnce, cancelUploads, stopRuntime)
				if drainErr != nil && !errors.Is(drainErr, context.DeadlineExceeded) && !errors.Is(drainErr, context.Canceled) {
					return drainErr
				}
				return nil
			}
			batchID, err := r.spoolEvent(queue, norm, r.currentDetection(), ev)
			if err != nil && !spool.IsBackpressure(err) {
				return err
			}
			if err != nil && spool.IsBackpressure(err) && r.Out != nil {
				stats, statErr := queue.Stats()
				if statErr != nil {
					return statErr
				}
				fmt.Fprintf(r.Out, "agent spool backpressure: dropped_batches=%d dropped_bytes=%d last_error=%q\n", stats.DroppedBatches, stats.DroppedBytes, stats.LastError)
			}
			if opts.DrainOnce {
				stats, err := worker.DrainOnce(ctx)
				if err != nil {
					return err
				}
				if r.Out != nil {
					fmt.Fprintf(r.Out, "agent upload drain: uploaded=%d remaining=%d last_error=%q\n", stats.UploadedBatches, stats.RemainingBatches, stats.LastError)
				}
			}
			if opts.Once {
				if r.Out != nil {
					fmt.Fprintf(r.Out, "agent daemon event: behavior=%s raw_ref=%s spool_batch=%s\n", ev.SensorEvent.GetBehavior(), ev.RawRef, batchID)
				}
				return nil
			}
		case <-ticker.C:
			health, err := r.collectHealth(ctx, rt, queue, worker, startedAt)
			if err != nil {
				return err
			}
			if sig := tamperDetector.Evaluate(health, time.Now().UTC(), tamper.Options{
				MaxRestarts:        uint64(r.Config.Sensor.MaxRestarts),
				MaxParseErrors:     r.Config.Sensor.MaxParseErrors,
				MaxDroppedEvents:   r.Config.Sensor.MaxDroppedEvents,
				NoEventGracePeriod: r.Config.Sensor.RestartWindow,
			}); sig != nil {
				batchID, err := r.spoolSignals(queue, []*signalv1.Signal{sig})
				if err != nil && !spool.IsBackpressure(err) {
					return err
				}
				if r.Out != nil {
					fmt.Fprintf(r.Out, "agent tamper signal: name=%s reason=%q spool_batch=%s\n", sig.GetName(), sig.GetEvidence().GetSummary(), batchID)
				}
			}
			if err := reporter.Report(ctx, health); err != nil && r.Out != nil {
				fmt.Fprintf(r.Out, "agent health report error: %v\n", err)
			}
			if r.Out != nil {
				fmt.Fprintf(r.Out, "agent health: sensor=%s running=%t policy_loaded=%t events_seen=%d queued_batches=%d queued_bytes=%d dropped_batches=%d last_spool_error=%q last_upload_error=%q\n",
					health.Sensor.Backend, health.Sensor.Running, health.Sensor.PolicyLoaded, health.Sensor.EventsSeen, health.Queue.QueuedBatches, health.Queue.QueuedBytes, health.Queue.DroppedBatches, health.Queue.LastError, health.Upload.LastError)
			}
			if responseClient != nil {
				if err := r.pollResponses(ctx, responseClient); err != nil && r.Out != nil {
					fmt.Fprintf(r.Out, "agent response poll error: %v\n", err)
				}
			}
			if streamResponseClient != nil {
				if err := r.pollStreamResponses(ctx, streamResponseClient); err != nil && r.Out != nil {
					fmt.Fprintf(r.Out, "agent stream response poll error: %v\n", err)
				}
			}
			if streamEvidenceClient != nil {
				if err := r.pollStreamEvidencePullbacks(ctx, streamEvidenceClient); err != nil && r.Out != nil {
					fmt.Fprintf(r.Out, "agent stream evidence pullback poll error: %v\n", err)
				}
			}
			if opts.Once {
				return nil
			}
		}
	}
}

func (r *Runner) shutdownAndReport(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *uploadworker.Worker, reporter healthReporter, startedAt time.Time, drainOnce bool, cancelUploads func(), stopRuntime func()) error {
	cancelUploads()
	stopRuntime()
	var drainErr error
	if !drainOnce {
		var stats uploadworker.Stats
		drainCtx, cancel := context.WithTimeout(context.Background(), shutdownDrainTimeout(r.Config))
		stats, drainErr = worker.DrainOnce(drainCtx)
		cancel()
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent shutdown drain: uploaded=%d remaining=%d last_error=%q\n", stats.UploadedBatches, stats.RemainingBatches, stats.LastError)
		}
	}
	finalHealth, healthErr := r.collectShutdownHealth(ctx, rt, queue, worker, startedAt)
	if healthErr == nil {
		if err := reporter.Report(context.Background(), finalHealth); err != nil && r.Out != nil {
			fmt.Fprintf(r.Out, "agent final health report error: %v\n", err)
		}
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent final health: sensor=%s running=%t policy_loaded=%t status=%s queued_batches=%d last_upload_error=%q\n",
				finalHealth.Sensor.Backend, finalHealth.Sensor.Running, finalHealth.Sensor.PolicyLoaded, finalHealth.Status, finalHealth.Queue.QueuedBatches, finalHealth.Upload.LastError)
		}
	}
	return drainErr
}

func (r *Runner) reportStartupFailure(reporter healthReporter, startedAt time.Time, stage string, startupErr error) {
	if reporter == nil || startupErr == nil {
		return
	}
	health := agenthealth.AgentHealth{
		AgentID:       r.Config.Agent.ID,
		HostID:        r.Config.Agent.HostID,
		TenantID:      r.Config.Agent.TenantID,
		Scope:         r.runtimeScope(),
		Status:        "degraded",
		PolicyID:      r.activePolicy().PolicyID,
		PolicyVersion: r.activePolicy().Version,
		PolicyMode:    r.policyMode(),
		UptimeSeconds: int64(time.Since(startedAt).Seconds()),
		ObservedAt:    time.Now().UTC(),
		Sensor: agenthealth.SensorHealth{
			Backend:      r.Config.Sensor.Backend,
			Installed:    false,
			Running:      false,
			PolicyLoaded: false,
			LastError:    fmt.Sprintf("%s: %v", stage, startupErr),
		},
		Capability: r.runtimeCapability(),
		Queue:      agenthealth.QueueHealth{},
		Upload:     agenthealth.UploadHealth{},
	}
	if err := reporter.Report(context.Background(), health); err != nil && r.Out != nil {
		fmt.Fprintf(r.Out, "agent startup health report error: %v\n", err)
	}
	if r.Out != nil {
		fmt.Fprintf(r.Out, "agent startup failure: stage=%s error=%q\n", stage, startupErr)
	}
}

func (r *Runner) healthReporter() healthReporter {
	if r.Config.Manager.Transport == "local" {
		return localHealthReporter{}
	}
	if r.Config.Manager.Transport == "stream" {
		return NewStreamHealthReporter(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout)
	}
	return agenthealth.NewReporter(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout)
}

func (r *Runner) collectShutdownHealth(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *uploadworker.Worker, startedAt time.Time) (agenthealth.AgentHealth, error) {
	health, err := r.collectHealth(ctx, rt, queue, worker, startedAt)
	if err != nil {
		return agenthealth.AgentHealth{}, err
	}
	health.Sensor.Running = false
	if health.Status == "ok" {
		health.Status = "degraded"
	}
	health.ObservedAt = time.Now().UTC()
	return health, nil
}

func shutdownDrainTimeout(cfg config.Config) time.Duration {
	timeout := cfg.Upload.RequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if cfg.Upload.RetryInitial > timeout {
		timeout = cfg.Upload.RetryInitial
	}
	return timeout
}

func (r *Runner) collectHealth(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *uploadworker.Worker, startedAt time.Time) (agenthealth.AgentHealth, error) {
	sensor, err := rt.Health(ctx)
	if err != nil {
		return agenthealth.AgentHealth{}, err
	}
	queueStats, err := queue.Stats()
	if err != nil {
		return agenthealth.AgentHealth{}, err
	}
	uploadStats, err := worker.Stats()
	if err != nil {
		return agenthealth.AgentHealth{}, err
	}
	status := "ok"
	if !sensor.Running || sensor.LastError != "" || queueStats.LastError != "" || uploadStats.LastError != "" {
		status = "degraded"
	}
	if queueStats.BackpressureCount > 0 || queueStats.DroppedBatches > 0 || queueStats.DroppedBytes > 0 {
		status = "degraded"
	}
	if r.Config.Sensor.MaxParseErrors > 0 && sensor.ParseErrors > r.Config.Sensor.MaxParseErrors {
		status = "degraded"
	}
	if r.Config.Sensor.MaxDroppedEvents > 0 && sensor.EventsDropped > r.Config.Sensor.MaxDroppedEvents {
		status = "degraded"
	}
	cepMetrics := r.currentDetection().Metrics()
	cepDegraded := cepMetrics.EvictedCEPGroups > 0 || cepMetrics.DroppedEventRefs > 0 || cepMetrics.CEPEvalErrors > 0
	if cepDegraded {
		status = "degraded"
	}
	now := time.Now().UTC()
	return agenthealth.AgentHealth{
		AgentID:       r.Config.Agent.ID,
		HostID:        r.Config.Agent.HostID,
		TenantID:      r.Config.Agent.TenantID,
		Scope:         r.runtimeScope(),
		Status:        status,
		PolicyID:      r.activePolicy().PolicyID,
		PolicyVersion: r.activePolicy().Version,
		PolicyMode:    r.policyMode(),
		UptimeSeconds: int64(time.Since(startedAt).Seconds()),
		ObservedAt:    now,
		Sensor: agenthealth.SensorHealth{
			Backend:        sensor.Backend,
			Installed:      sensor.Installed,
			Running:        sensor.Running,
			Version:        sensor.Version,
			PolicyLoaded:   sensor.PolicyLoaded,
			EventsSeen:     sensor.EventsSeen,
			EventsDropped:  sensor.EventsDropped,
			ParseErrors:    sensor.ParseErrors,
			RestartCount:   sensor.RestartCount,
			LastEventAt:    sensor.LastEventAt,
			LastExitReason: sensor.LastExitReason,
			LastError:      sensor.LastError,
		},
		Capability: r.runtimeCapability(),
		Queue: agenthealth.QueueHealth{
			QueuedBatches:     queueStats.QueuedBatches,
			QueuedBytes:       queueStats.QueuedBytes,
			MaxBytes:          queueStats.MaxBytes,
			BackpressureCount: queueStats.BackpressureCount,
			DroppedBatches:    queueStats.DroppedBatches,
			DroppedBytes:      queueStats.DroppedBytes,
			LastError:         queueStats.LastError,
		},
		Upload: agenthealth.UploadHealth{
			UploadedBatches:  uploadStats.UploadedBatches,
			RemainingBatches: uploadStats.RemainingBatches,
			RemainingBytes:   uploadStats.RemainingBytes,
			LastError:        uploadStats.LastError,
		},
		CEP: agenthealth.CEPHealth{
			ActiveGroups:     cepMetrics.ActiveCEPGroups,
			EvictedGroups:    cepMetrics.EvictedCEPGroups,
			ExpiredGroups:    cepMetrics.ExpiredCEPGroups,
			DroppedEventRefs: cepMetrics.DroppedEventRefs,
			EvalErrors:       cepMetrics.CEPEvalErrors,
			EmittedSignals:   cepMetrics.EmittedSignals,
			Degraded:         cepDegraded,
		},
	}, nil
}

func (r *Runner) runtimeCapability() agenthealth.SensorCapability {
	collection := make([]agenthealth.CollectionBehaviorCapability, 0, len(r.capability.Collection))
	for _, behavior := range r.capability.Collection {
		collection = append(collection, agenthealth.CollectionBehaviorCapability{
			Behavior:             behavior.Behavior,
			SensorMapping:        behavior.SensorMapping,
			Fields:               append([]string(nil), behavior.Fields...),
			PushdownSelectors:    append([]string(nil), behavior.PushdownSelectors...),
			AgentSideSelectors:   append([]string(nil), behavior.AgentSideSelectors...),
			UnsupportedSelectors: append([]string(nil), behavior.UnsupportedSelectors...),
		})
	}
	return agenthealth.SensorCapability{
		Backend:         r.capability.Backend,
		Version:         r.capability.Version,
		SupportsExec:    r.capability.SupportsExec,
		SupportsConnect: r.capability.SupportsConnect,
		SupportsFile:    r.capability.SupportsFile,
		SupportsEnforce: r.capability.SupportsEnforce,
		SupportsHealth:  r.capability.SupportsHealth,
		KernelRelease:   r.capability.KernelRelease,
		BTFAvailable:    r.capability.BTFAvailable,
		BPFFSAvailable:  r.capability.BPFFSAvailable,
		Collection:      collection,
	}
}

func (r *Runner) runtimeScope() agenthealth.RuntimeScope {
	scope, err := r.Config.Sensor.EffectiveScope()
	if err != nil {
		return agenthealth.RuntimeScope{Type: "host"}
	}
	return agenthealth.RuntimeScope{Type: scope.Type, Selector: scope.Selector}
}

func (r *Runner) policyMode() string {
	if mode := r.activePolicy().Mode; mode != "" {
		return mode
	}
	if r.Config.Sensor.ObserveOnly {
		return "observe"
	}
	return "enforce"
}

func (r *Runner) fetchStartupPolicy(ctx context.Context, scopeType, scopeSelector string) policymodel.Policy {
	defaultPolicy := policymodel.DefaultPolicy(r.Config.Agent.TenantID)
	r.setPolicy(defaultPolicy)
	if r.Config.Manager.Transport != "http" && r.Config.Manager.Transport != "stream" {
		return defaultPolicy
	}
	policy, err := r.effectivePolicy(ctx, EffectivePolicyRequest{
		TenantID:      r.Config.Agent.TenantID,
		AgentID:       r.Config.Agent.ID,
		ScopeType:     scopeType,
		ScopeSelector: scopeSelector,
	})
	if err != nil {
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent policy fetch error: %v; using default policy\n", err)
		}
		return defaultPolicy
	}
	policy = policymodel.Normalize(policy)
	r.setPolicy(policy)
	return policy
}

func (r *Runner) activePolicy() policymodel.Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.policy.PolicyID != "" {
		return r.policy
	}
	return policymodel.DefaultPolicy(r.Config.Agent.TenantID)
}

func (r *Runner) setPolicy(policy policymodel.Policy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy = policy
}

func (r *Runner) currentDetection() *detection.Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.detection != nil {
		return r.detection
	}
	engine, _ := detection.NewWithRuntimeLimits(policymodel.DefaultDetectionPolicy(), r.collection, detection.ContentSnapshot{}, r.detectionLimits())
	return engine
}

func (r *Runner) setDetection(engine *detection.Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.detection = engine
}

func (r *Runner) setCollectionIntent(intent contract.CollectionIntent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collection = r.withCollectionCapabilities(intent)
}

func (r *Runner) currentCollectionIntent() contract.CollectionIntent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.collection
}

func (r *Runner) withCollectionCapabilities(intent contract.CollectionIntent) contract.CollectionIntent {
	if len(intent.Capabilities) == 0 && len(r.capability.Collection) > 0 {
		intent.Capabilities = append([]contract.CollectionBehaviorCapability(nil), r.capability.Collection...)
	}
	return intent
}

func (r *Runner) runPolicyRefreshLoop(ctx context.Context, scopeType, scopeSelector string) {
	interval := r.Config.Policy.RefreshInterval
	if interval <= 0 {
		return
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			changed, err := r.refreshPolicy(ctx, scopeType, scopeSelector)
			if err != nil && r.Out != nil {
				fmt.Fprintf(r.Out, "agent policy refresh error: %v\n", err)
			}
			if changed && r.Out != nil {
				policy := r.activePolicy()
				fmt.Fprintf(r.Out, "agent policy refreshed: policy=%s version=%d mode=%s\n", policy.PolicyID, policy.Version, policy.Mode)
			}
			timer.Reset(interval)
		}
	}
}

func (r *Runner) refreshPolicy(ctx context.Context, scopeType, scopeSelector string) (bool, error) {
	policy, err := r.effectivePolicy(ctx, EffectivePolicyRequest{
		TenantID:      r.Config.Agent.TenantID,
		AgentID:       r.Config.Agent.ID,
		ScopeType:     scopeType,
		ScopeSelector: scopeSelector,
	})
	if err != nil {
		return false, err
	}
	policy = policymodel.Normalize(policy)
	current := r.activePolicy()
	if samePolicyRuntime(current, policy) {
		return false, nil
	}
	r.applyRuntimePolicy(policy)
	return true, nil
}

func (r *Runner) effectivePolicy(ctx context.Context, req EffectivePolicyRequest) (policymodel.Policy, error) {
	switch r.Config.Manager.Transport {
	case "http":
		return NewPolicyClient(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout).EffectivePolicy(ctx, req)
	case "stream":
		return NewStreamPolicyClient(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout).EffectivePolicy(ctx, req)
	default:
		return policymodel.Policy{}, fmt.Errorf("policy fetch unsupported for transport %q", r.Config.Manager.Transport)
	}
}

func (r *Runner) applyRuntimePolicy(policy policymodel.Policy) {
	policy = policymodel.Normalize(policy)
	engine, _ := detection.NewWithRuntimeLimits(policy.Detection, r.currentCollectionIntent(), r.detectionContentSnapshot(), r.detectionLimits())
	r.setPolicy(policy)
	r.setDetection(engine)
}

func (r *Runner) rebuildDetection() detection.ApplyReport {
	policy := policymodel.Normalize(r.activePolicy())
	engine, report := detection.NewWithRuntimeLimits(policy.Detection, r.currentCollectionIntent(), r.detectionContentSnapshot(), r.detectionLimits())
	r.setDetection(engine)
	return report
}

func (r *Runner) detectionLimits() detection.EngineLimits {
	return detection.EngineLimits{
		MaxCEPGroups: r.Config.Resource.MaxActiveCEPGroups,
		MaxCEPRefs:   r.Config.Resource.MaxEventRefsPerSignal,
	}
}

func samePolicyRuntime(a, b policymodel.Policy) bool {
	return a.PolicyID == b.PolicyID &&
		a.Version == b.Version &&
		a.Mode == b.Mode &&
		reflect.DeepEqual(a.Detection, b.Detection)
}

func runUploadLoop(ctx context.Context, worker *uploadworker.Worker, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			_, _ = worker.DrainWithRetry(ctx)
			timer.Reset(interval)
		}
	}
}

func (r *Runner) uploadWorker(queue *spool.Queue) (*uploadworker.Worker, error) {
	up, err := newBatchUploader(r.Config.Manager.Address, r.Config.Manager.Transport, r.Config.Upload.RequestTimeout, r.Config.Agent.Token)
	if err != nil {
		return nil, err
	}
	worker := &uploadworker.Worker{
		Queue:    queue,
		Uploader: up,
		Backoff:  uploadworker.Backoff{Initial: r.Config.Upload.RetryInitial, Max: r.Config.Upload.RetryMax},
	}
	switch r.Config.Manager.Transport {
	case "http":
		worker.ResumeSource = NewResumeClient(
			r.Config.Manager.Address,
			r.Config.Agent.Token,
			r.Config.Upload.RequestTimeout,
			r.Config.Agent.TenantID,
			r.Config.Agent.ID,
		)
	case "stream":
		worker.ResumeSource = NewStreamResumeClient(
			r.Config.Manager.Address,
			r.Config.Agent.Token,
			r.Config.Upload.RequestTimeout,
			r.Config.Agent.TenantID,
			r.Config.Agent.ID,
		)
	}
	return worker, nil
}

func newBatchUploader(manager, transport string, timeout time.Duration, token string) (uploader.BatchUploader, error) {
	switch transport {
	case "http":
		return uploader.NewHTTPUploaderWithOptions(manager, timeout, token), nil
	case "grpc":
		return uploader.NewGRPCUploaderWithOptions(manager, timeout, token), nil
	case "stream":
		return uploader.NewStreamUploaderWithOptions(manager, timeout, token), nil
	case "local":
		return localBatchUploader{}, nil
	default:
		return nil, fmt.Errorf("unknown transport %q", transport)
	}
}

func parseTrustKeys(raw string) map[string]ed25519.PublicKey {
	out := map[string]ed25519.PublicKey{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		keyID, encoded, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		keyID = strings.TrimSpace(keyID)
		encoded = strings.TrimSpace(encoded)
		if keyID == "" || encoded == "" {
			continue
		}
		if data, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(data) == ed25519.PublicKeySize {
			out[keyID] = ed25519.PublicKey(data)
		}
	}
	return out
}

func (r *Runner) spoolEvent(queue *spool.Queue, norm *normalize.Normalizer, detector *detection.Engine, ev contract.EventEnvelope) (string, error) {
	if ev.SensorEvent == nil {
		return "", fmt.Errorf("sensor event is nil")
	}
	if ev.SensorEvent.RawRef == "" {
		ev.SensorEvent.RawRef = ev.RawRef
	}
	canonical := norm.Normalize(ev.SensorEvent)
	if canonical.Scenario == "" {
		canonical.Scenario = r.Config.Agent.Scenario
	}
	signals := detector.Process(canonical)
	for _, sig := range signals {
		if sig.Scenario == "" {
			sig.Scenario = r.Config.Agent.Scenario
		}
	}
	r.localStreams().publishEvent(r.Config.Agent.TenantID, r.Config.Agent.ID, canonical)
	r.localStreams().publishSignals(r.Config.Agent.TenantID, r.Config.Agent.ID, signals)
	batch := &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{
			AgentId:  r.Config.Agent.ID,
			HostId:   r.Config.Agent.HostID,
			TenantId: r.Config.Agent.TenantID,
			Version:  "dev",
		},
		Events:  []*eventv1.CanonicalEvent{canonical},
		Signals: signals,
	}
	return queue.Append(batch)
}

func (r *Runner) spoolSignals(queue *spool.Queue, signals []*signalv1.Signal) (string, error) {
	r.localStreams().publishSignals(r.Config.Agent.TenantID, r.Config.Agent.ID, signals)
	batch := &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{
			AgentId:  r.Config.Agent.ID,
			HostId:   r.Config.Agent.HostID,
			TenantId: r.Config.Agent.TenantID,
			Version:  "dev",
		},
		Signals: signals,
	}
	return queue.Append(batch)
}

func (r *Runner) localStreams() *localStreamBuffer {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.streams == nil {
		r.streams = newLocalStreamBuffer(defaultLocalStreamCapacity)
	}
	return r.streams
}

func (r *Runner) contentStore() *agentcontent.Store {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.content == nil {
		r.content = agentcontent.NewStore()
	}
	return r.content
}

func (r *Runner) detectionContentSnapshot() detection.ContentSnapshot {
	snapshot := r.contentStore().Snapshot()
	out := detection.ContentSnapshot{
		ContextRefs: make(map[string]detection.ContentRef),
		IOCRefs:     make(map[string]detection.ContentRef),
	}
	for ref, set := range snapshot.ContextSets {
		out.ContextRefs[ref] = detection.ContentRef{
			Ref:     set.Ref,
			Version: set.Version,
			Digest:  set.Digest,
			Values:  append([]string(nil), set.Values...),
		}
	}
	for ref, set := range snapshot.IOCPacks {
		out.IOCRefs[ref] = detection.ContentRef{
			Ref:     set.Ref,
			Version: set.Version,
			Digest:  set.Digest,
			Values:  append([]string(nil), set.Values...),
		}
	}
	for _, rule := range snapshot.Rules {
		out.Rules = append(out.Rules, detection.RuleSpec{
			RuleID:            rule.RuleID,
			Version:           rule.Version,
			RuleSetRef:        rule.RuleSetRef,
			Severity:          rule.Severity,
			Runtime:           rule.RuntimeEntry,
			RuntimeType:       rule.RuntimeType,
			Expr:              detectionExpr(rule.Expr),
			Sequence:          detectionSequence(rule.Sequence),
			RequiredEvents:    detectionRequiredEvents(rule.RequiredEvents),
			RequiredBehaviors: requiredBehaviors(rule.RequiredEvents),
			ContextRefs:       append([]string(nil), rule.ContextRefs...),
			IOCRefs:           append([]string(nil), rule.IOCRefs...),
			ResponseIntent: &policymodel.ResponseIntentRef{
				Action:     rule.ResponseIntent.Action,
				Confidence: rule.ResponseIntent.Confidence,
				Reason:     rule.ResponseIntent.Reason,
			},
		})
	}
	return out
}

func detectionRequiredEvents(events []agentcontent.RequiredEvent) []detection.RequiredEventSpec {
	out := make([]detection.RequiredEventSpec, 0, len(events))
	for _, event := range events {
		out = append(out, detection.RequiredEventSpec{
			Behavior: event.Behavior,
			Fields:   append([]string(nil), event.Fields...),
		})
	}
	return out
}

func detectionExpr(expr agentcontent.RuntimeExpr) detection.ExprSpec {
	out := detection.ExprSpec{Conditions: make([]detection.ConditionSpec, 0, len(expr.Conditions))}
	for _, cond := range expr.Conditions {
		out.Conditions = append(out.Conditions, detectionCondition(cond))
	}
	return out
}

func detectionSequence(seq agentcontent.RuntimeSequence) detection.SequenceSpec {
	within, _ := time.ParseDuration(seq.Within)
	out := detection.SequenceSpec{
		Within: within,
		By:     append([]string(nil), seq.By...),
		Steps:  make([]detection.StepSpec, 0, len(seq.Steps)),
	}
	for _, step := range seq.Steps {
		behavior := step.Behavior
		if behavior == "" {
			behavior = step.Event
		}
		next := detection.StepSpec{
			ID:         step.ID,
			Behavior:   eventmodel.NormalizeBehavior(behavior).String(),
			Conditions: make([]detection.ConditionSpec, 0, len(step.Conditions)),
		}
		for _, cond := range step.Conditions {
			next.Conditions = append(next.Conditions, detectionCondition(cond))
		}
		out.Steps = append(out.Steps, next)
	}
	return out
}

func detectionCondition(cond agentcontent.RuntimeCondition) detection.ConditionSpec {
	return detection.ConditionSpec{
		Field:     cond.Field,
		Op:        cond.Op,
		Value:     cond.Value,
		Values:    append([]string(nil), cond.Values...),
		Ref:       cond.Ref,
		Step:      cond.Step,
		StepField: cond.StepField,
	}
}

func requiredBehaviors(events []agentcontent.RequiredEvent) []string {
	seen := map[string]bool{}
	var out []string
	for _, event := range events {
		behavior := eventmodel.NormalizeBehavior(event.Behavior).String()
		if behavior == "" || seen[behavior] {
			continue
		}
		seen[behavior] = true
		out = append(out, behavior)
	}
	return out
}

func (r *Runner) pollResponses(ctx context.Context, client *ResponseClient) error {
	commands, err := client.Pending(ctx, r.Config.Agent.TenantID, r.Config.Agent.ID)
	if err != nil {
		return err
	}
	for _, cmd := range commands {
		ack := r.executeResponse(ctx, cmd)
		if err := client.Ack(ctx, ack); err != nil {
			return err
		}
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent response ack: response=%s action=%s observe_only=%t unsupported=%t executed=%t\n", ack.ResponseID, cmd.Action, ack.ObserveOnly, ack.Unsupported, ack.Executed)
		}
	}
	return nil
}

func (r *Runner) pollStreamResponses(ctx context.Context, client *StreamResponseClient) error {
	commands, err := client.Pending(ctx, r.Config.Agent.TenantID, r.Config.Agent.ID)
	if err != nil {
		return err
	}
	for _, cmd := range commands {
		ack := r.executeResponse(ctx, cmd)
		if err := client.Ack(ctx, ack); err != nil {
			return err
		}
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent stream response ack: response=%s action=%s observe_only=%t unsupported=%t executed=%t\n", ack.ResponseID, cmd.Action, ack.ObserveOnly, ack.Unsupported, ack.Executed)
		}
	}
	return nil
}

func (r *Runner) executeResponse(ctx context.Context, cmd responsemodel.Command) responsemodel.Ack {
	cmd.Mode = responsemodel.DefaultMode
	ack, err := r.Sensor.Enforce(ctx, responsemodel.ToEnforcement(cmd))
	if err != nil {
		ack = contract.UnsupportedAck(responsemodel.ToEnforcement(cmd), err.Error())
	}
	out := responsemodel.FromEnforcementAck(cmd, ack)
	out.TenantID = r.Config.Agent.TenantID
	out.AgentID = r.Config.Agent.ID
	out.ObserveOnly = true
	out.Executed = false
	return out
}

func (r *Runner) pollStreamEvidencePullbacks(ctx context.Context, client *StreamEvidenceClient) error {
	requests, err := client.Pending(ctx, r.Config.Agent.TenantID, r.Config.Agent.ID)
	if err != nil {
		return err
	}
	for _, req := range requests {
		result := r.collectEvidencePullback(req)
		if err := client.Result(ctx, result); err != nil {
			return err
		}
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent stream evidence pullback result: request=%s ok=%t message=%q\n", result.RequestID, result.OK, result.Message)
		}
	}
	return nil
}

func (r *Runner) collectEvidencePullback(req gatewaymodel.EvidencePullbackRequest) gatewaymodel.EvidencePullbackResult {
	result := gatewaymodel.EvidencePullbackResult{
		RequestID:  req.RequestID,
		TenantID:   r.Config.Agent.TenantID,
		AgentID:    r.Config.Agent.ID,
		OK:         true,
		Message:    "collected target evidence",
		ObservedAt: time.Now().UTC(),
	}
	if req.Target == "" {
		result.Message = "collected no target evidence"
		return result
	}
	evidence := &incidentv1.EvidenceSubgraph{
		Nodes: []*incidentv1.GraphNode{{
			Id:    req.Target,
			Kind:  evidenceKindFromTarget(req.Target),
			Label: req.Target,
		}},
	}
	data, err := protojson.Marshal(evidence)
	if err != nil {
		result.OK = false
		result.Message = fmt.Sprintf("encode evidence: %v", err)
		return result
	}
	result.Evidence = json.RawMessage(data)
	return result
}

func evidenceKindFromTarget(target string) string {
	if idx := strings.Index(target, ":"); idx > 0 {
		return target[:idx]
	}
	return "entity"
}

func sensorFromConfig(cfg config.Config) (contract.Sensor, error) {
	switch cfg.Sensor.Backend {
	case "fake":
		count := cfg.Sensor.FakeStartupEvents
		if count == 0 {
			count = 1
		}
		return fake.NewWithStartupEvents(count), nil
	case "tetragon":
		restart, err := tetragonRestartPolicy(cfg.Sensor)
		if err != nil {
			return nil, err
		}
		backend := tetragon.NewBackendWithOptions(cfg.Sensor.PolicyPath, cfg.Sensor.EventSource, cfg.Sensor.Version, tetragon.BundleConfig{
			BundleDir:    cfg.Sensor.BundleDir,
			InstallDir:   cfg.Sensor.InstallDir,
			TetraPath:    cfg.Sensor.TetraPath,
			TetragonPath: cfg.Sensor.TetragonPath,
		}, restart)
		scope, err := cfg.Sensor.EffectiveScope()
		if err != nil {
			return nil, err
		}
		backend.ScopeType = scope.Type
		backend.ScopeSelector = scope.Selector
		backend.ContainerIDPrefix = cfg.Sensor.ContainerIDPrefix
		backend.BTFPath = cfg.Sensor.BTFPath
		backend.BPFFSPath = cfg.Sensor.BPFFSPath
		backend.RequireBTF = cfg.Sensor.RequireBTF
		backend.RequireBPFFS = cfg.Sensor.RequireBPFFS
		return backend, nil
	default:
		return nil, fmt.Errorf("unsupported sensor backend %q", cfg.Sensor.Backend)
	}
}

func tetragonRestartPolicy(cfg config.SensorConfig) (tetragon.ProcessRestartPolicy, error) {
	switch cfg.Restart {
	case "", "never", "off", "false":
		return tetragon.ProcessRestartPolicy{}, nil
	case "always":
		return tetragon.ProcessRestartPolicy{
			Enabled:     true,
			MaxRestarts: cfg.MaxRestarts,
			Delay:       cfg.RestartWindow,
		}, nil
	default:
		return tetragon.ProcessRestartPolicy{}, fmt.Errorf("unsupported sensor.restart %q", cfg.Restart)
	}
}
