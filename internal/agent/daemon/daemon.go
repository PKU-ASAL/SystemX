package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/tamper"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/uploadworker"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/fastpath"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/uploader"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/fake"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/tetragon"
)

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
	fastpath   *fastpath.Engine
}

func New(cfg config.Config) (*Runner, error) {
	sensor, err := sensorFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Runner{Config: cfg, Sensor: sensor}, nil
}

func (r *Runner) Run(ctx context.Context, opts Options) error {
	if opts.Out != nil {
		r.Out = opts.Out
	}
	startedAt := time.Now()
	reporter := agenthealth.NewReporter(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout)
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
	if r.Config.Manager.Transport == "http" {
		stats, err := worker.ResumeOnce(ctx)
		if err != nil {
			return failStartup("resume", err)
		}
		if r.Out != nil && stats.LastError != "" {
			fmt.Fprintf(r.Out, "agent link1 resume error: %s\n", stats.LastError)
		}
	}
	var responseClient *ResponseClient
	if r.Config.Manager.Transport == "http" {
		responseClient = NewResponseClient(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout)
	}
	var streamResponseClient *StreamResponseClient
	if r.Config.Manager.Transport == "stream" {
		streamResponseClient = NewStreamResponseClient(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout)
	}
	uploadCtx := ctx
	cancelUploads := func() {}
	if !opts.Once && !opts.DrainOnce {
		uploadCtx, cancelUploads = context.WithCancel(ctx)
		defer cancelUploads()
		go runUploadLoop(uploadCtx, worker, r.Config.Spool.FlushInterval)
	}
	norm := normalize.New(r.Config.Agent.ID, r.Config.Agent.HostID, nil)
	r.setFastpath(fastpath.NewWithRules(effectivePolicy.EndpointRules))
	refreshCtx := ctx
	cancelRefresh := func() {}
	if !opts.Once && r.Config.Policy.RefreshInterval > 0 && (r.Config.Manager.Transport == "http" || r.Config.Manager.Transport == "stream") {
		refreshCtx, cancelRefresh = context.WithCancel(ctx)
		defer cancelRefresh()
		go r.runPolicyRefreshLoop(refreshCtx, scopeType, scopeSelector)
	}
	if r.Out != nil {
		fmt.Fprintf(r.Out, "agent daemon started: agent=%s host=%s tenant=%s sensor=%s version=%s kinds=%d policy=%s version=%d mode=%s\n",
			r.Config.Agent.ID, r.Config.Agent.HostID, r.Config.Agent.TenantID, capability.Backend, capability.Version, len(intent.EventKinds), effectivePolicy.PolicyID, effectivePolicy.Version, effectivePolicy.Mode)
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
			batchID, err := r.spoolEvent(queue, norm, r.currentFastpath(), ev)
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
					fmt.Fprintf(r.Out, "agent daemon event: kind=%s raw_ref=%s spool_batch=%s\n", ev.SensorEvent.GetKind().String(), ev.RawRef, batchID)
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
			if opts.Once {
				return nil
			}
		}
	}
}

func (r *Runner) shutdownAndReport(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *uploadworker.Worker, reporter *agenthealth.Reporter, startedAt time.Time, drainOnce bool, cancelUploads func(), stopRuntime func()) error {
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

func (r *Runner) reportStartupFailure(reporter *agenthealth.Reporter, startedAt time.Time, stage string, startupErr error) {
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
	}, nil
}

func (r *Runner) runtimeCapability() agenthealth.SensorCapability {
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

func (r *Runner) currentFastpath() *fastpath.Engine {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.fastpath != nil {
		return r.fastpath
	}
	return fastpath.New()
}

func (r *Runner) setFastpath(engine *fastpath.Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fastpath = engine
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
	r.setPolicy(policy)
	r.setFastpath(fastpath.NewWithRules(policy.EndpointRules))
}

func samePolicyRuntime(a, b policymodel.Policy) bool {
	return a.PolicyID == b.PolicyID &&
		a.Version == b.Version &&
		a.Mode == b.Mode &&
		reflect.DeepEqual(a.EndpointRules, b.EndpointRules)
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
	if r.Config.Manager.Transport == "http" {
		worker.ResumeSource = NewResumeClient(
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
	default:
		return nil, fmt.Errorf("unknown transport %q", transport)
	}
}

func (r *Runner) spoolEvent(queue *spool.Queue, norm *normalize.Normalizer, fp *fastpath.Engine, ev contract.EventEnvelope) (string, error) {
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
	signals := fp.Process(canonical)
	for _, sig := range signals {
		if sig.Scenario == "" {
			sig.Scenario = r.Config.Agent.Scenario
		}
	}
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
