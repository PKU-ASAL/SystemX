package daemon

import (
	"context"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/uploadworker"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
)

type TransportRuntime struct {
	runner        *Runner
	sensor        sensorruntime.Runtime
	spool         *AgentSpool
	worker        *uploadworker.Worker
	startedAt     time.Time
	scopeType     string
	scopeSelector string
}

func NewTransportRuntime(runner *Runner, sensor sensorruntime.Runtime, spool *AgentSpool, worker *uploadworker.Worker, startedAt time.Time, scopeType, scopeSelector string) *TransportRuntime {
	return &TransportRuntime{
		runner:        runner,
		sensor:        sensor,
		spool:         spool,
		worker:        worker,
		startedAt:     startedAt,
		scopeType:     scopeType,
		scopeSelector: scopeSelector,
	}
}

func (r *TransportRuntime) RunDataFlow(ctx context.Context) {
	if r == nil || r.runner == nil {
		return
	}
	r.runner.runTransportDataLoop(ctx, r.worker, r.runner.Config.Spool.FlushInterval)
}

func (r *TransportRuntime) RunControlFlow(ctx context.Context) {
	if r == nil || r.runner == nil || r.runner.Config.Manager.Transport != "grpc" {
		return
	}
	r.runner.runTransportControlLoop(ctx, r.sensor, r.spool, r.worker, r.startedAt, r.scopeType, r.scopeSelector)
}
