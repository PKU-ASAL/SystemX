package daemon

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/config"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/policy"
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
			if opts.Once {
				if r.Out != nil {
					fmt.Fprintf(r.Out, "agent daemon event: kind=%s raw_ref=%s\n", ev.SensorEvent.GetKind().String(), ev.RawRef)
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
