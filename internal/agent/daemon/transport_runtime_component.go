package daemon

import (
	"context"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/uploadworker"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
)

type TransportRuntime struct {
	runner        *AgentRuntime
	sensor        sensorruntime.Runtime
	spool         *AgentSpool
	worker        *uploadworker.Worker
	startedAt     time.Time
	scopeType     string
	scopeSelector string
}

func NewTransportRuntime(runner *AgentRuntime, sensor sensorruntime.Runtime, spool *AgentSpool, worker *uploadworker.Worker, startedAt time.Time, scopeType, scopeSelector string) *TransportRuntime {
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
	interval := r.runner.Config.Spool.FlushInterval
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
			_, _ = r.worker.DrainWithRetry(ctx)
			timer.Reset(interval)
		}
	}
}

func (r *TransportRuntime) RunControlFlow(ctx context.Context) {
	if r == nil || r.runner == nil || r.runner.Config.Manager.Transport != "grpc" {
		return
	}
	r.runControlFlow(ctx)
}
