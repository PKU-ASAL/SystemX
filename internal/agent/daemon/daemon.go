package daemon

import (
	"context"
	"fmt"
	"io"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
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
	Config config.Config
	Sensor contract.Sensor
	Out    io.Writer
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
	rt := sensorruntime.New(r.Sensor)
	capability, err := rt.Probe(ctx)
	if err != nil {
		return err
	}
	intent, err := policy.LoadCollectionIntent(r.Config.Sensor.PolicyPath, r.Config.Sensor.ObserveOnly)
	if err != nil {
		return err
	}
	if err := rt.Apply(ctx, intent); err != nil {
		return err
	}
	events, err := rt.Subscribe(ctx)
	if err != nil {
		return err
	}
	queue, err := spool.OpenWithLimit(r.Config.Spool.Path, r.Config.Spool.MaxBytes)
	if err != nil {
		return err
	}
	worker, err := r.uploadWorker(queue)
	if err != nil {
		return err
	}
	if !opts.Once && !opts.DrainOnce {
		uploadCtx, cancelUploads := context.WithCancel(ctx)
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
	defer rt.Stop(context.Background())
	startedAt := time.Now()
	reporter := agenthealth.NewReporter(r.Config.Manager.Address, r.Config.Agent.Token, r.Config.Upload.RequestTimeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
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
	now := time.Now().UTC()
	return agenthealth.AgentHealth{
		AgentID:       r.Config.Agent.ID,
		HostID:        r.Config.Agent.HostID,
		TenantID:      r.Config.Agent.TenantID,
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
	signals := fp.Process(canonical)
	batch := &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{
			AgentId: r.Config.Agent.ID,
			HostId:  r.Config.Agent.HostID,
			Version: "dev",
		},
		Events:  []*eventv1.CanonicalEvent{canonical},
		Signals: signals,
	}
	return queue.Append(batch)
}

func sensorFromConfig(cfg config.Config) (contract.Sensor, error) {
	switch cfg.Sensor.Backend {
	case "fake":
		return fake.New(), nil
	case "tetragon":
		return tetragon.NewBackend(cfg.Sensor.PolicyPath, cfg.Sensor.EventSource, cfg.Sensor.Version), nil
	default:
		return nil, fmt.Errorf("unsupported sensor backend %q", cfg.Sensor.Backend)
	}
}
