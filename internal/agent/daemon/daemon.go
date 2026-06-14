package daemon

import (
	"context"
	"fmt"
	"io"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/fastpath"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/fake"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/tetragon"
)

type Options struct {
	Once bool
	Out  io.Writer
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
	queue, err := spool.Open(r.Config.Spool.Path)
	if err != nil {
		return err
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

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-events:
			if !ok {
				return nil
			}
			batchID, err := r.spoolEvent(queue, norm, fp, ev)
			if err != nil {
				return err
			}
			if opts.Once {
				if r.Out != nil {
					fmt.Fprintf(r.Out, "agent daemon event: kind=%s raw_ref=%s spool_batch=%s\n", ev.SensorEvent.GetKind().String(), ev.RawRef, batchID)
				}
				return nil
			}
		case <-ticker.C:
			health, err := rt.Health(ctx)
			if err != nil {
				return err
			}
			if r.Out != nil {
				fmt.Fprintf(r.Out, "agent health: sensor=%s running=%t policy_loaded=%t events_seen=%d\n",
					health.Backend, health.Running, health.PolicyLoaded, health.EventsSeen)
			}
			if opts.Once {
				return nil
			}
		}
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
