package daemon

import (
	"context"
	"sync"
	"time"

	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/localapi"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/sensors/runtime"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/telemetry"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func (r *AgentRuntime) startLocalControlServer(ctx context.Context, rt sensorruntime.Runtime, source any, rest ...any) (func(), error) {
	coordinator := r.configureEnrollmentCoordinator(ctx, rt)
	bus, batcher, sender, startedAt := r.localControlTelemetryArgs(source, rest...)
	socketPath := r.Config.Control.SocketPath
	handler := &localControlServer{
		runner:     r,
		enrollment: coordinator,
		runtime:    rt,
		bus:        bus,
		batcher:    batcher,
		sender:     sender,
		startedAt:  startedAt,
	}
	return localapi.New(socketPath, handler, r.Out).Start(ctx)
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
			batcher = telemetry.NewBatcher(r.newDataBatch, r.Config.Telemetry.MaxBatchItems, r.Config.Telemetry.FlushInterval, 64, r.Config.Telemetry.MaxBatchBytes)
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
	bus := telemetry.NewBus(r.Config.Telemetry.MaxBatchItems * 16)
	batcher := telemetry.NewBatcher(r.newDataBatch, r.Config.Telemetry.MaxBatchItems, r.Config.Telemetry.FlushInterval, 64, r.Config.Telemetry.MaxBatchBytes)
	sender := &telemetry.Sender{Appender: localBatchSender{}, Batcher: batcher}
	startedAt := time.Now().UTC()
	return bus, batcher, sender, startedAt
}

type localControlServer struct {
	controlplanev1.UnimplementedAgentControlPlaneServiceServer
	runner     *AgentRuntime
	enrollment *enrollmentCoordinator
	runtime    sensorruntime.Runtime
	bus        *telemetry.Bus
	batcher    *telemetry.Batcher
	sender     *telemetry.Sender
	startedAt  time.Time
	profileMu  sync.Mutex
}

func (r *AgentRuntime) configureEnrollmentCoordinator(ctx context.Context, rt sensorruntime.Runtime) *enrollmentCoordinator {
	r.enrollmentCoordinatorMu.Lock()
	defer r.enrollmentCoordinatorMu.Unlock()
	if r.enrollmentCoordinator == nil {
		r.enrollmentCoordinator = newEnrollmentCoordinator(ctx, r, rt)
	}
	return r.enrollmentCoordinator
}

func (s *localControlServer) enrollmentCoordinator(ctx context.Context) *enrollmentCoordinator {
	if s.enrollment != nil {
		return s.enrollment
	}
	return s.runner.configureEnrollmentCoordinator(ctx, s.runtime)
}
