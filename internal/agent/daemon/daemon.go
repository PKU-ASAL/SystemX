package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	uploadCtx := ctx
	cancelUploads := func() {}
	if !opts.Once && !opts.DrainOnce {
		uploadCtx, cancelUploads = context.WithCancel(ctx)
		defer cancelUploads()
		go runUploadLoop(uploadCtx, worker, r.Config.Spool.FlushInterval)
	}
	norm := normalize.New(r.Config.Agent.ID, r.Config.Agent.HostID, nil)
	fp := fastpath.New()
	if r.Out != nil {
		fmt.Fprintf(r.Out, "agent daemon started: agent=%s host=%s tenant=%s sensor=%s version=%s kinds=%d\n",
			r.Config.Agent.ID, r.Config.Agent.HostID, r.Config.Agent.TenantID, capability.Backend, capability.Version, len(intent.EventKinds))
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
			batchID, err := r.spoolEvent(queue, norm, fp, ev)
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
	return &uploadworker.Worker{
		Queue:    queue,
		Uploader: up,
		Backoff:  uploadworker.Backoff{Initial: r.Config.Upload.RetryInitial, Max: r.Config.Upload.RetryMax},
	}, nil
}

func newBatchUploader(manager, transport string, timeout time.Duration, token string) (uploader.BatchUploader, error) {
	switch transport {
	case "http":
		return uploader.NewHTTPUploaderWithOptions(manager, timeout, token), nil
	case "grpc":
		return uploader.NewGRPCUploaderWithOptions(manager, timeout, token), nil
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
