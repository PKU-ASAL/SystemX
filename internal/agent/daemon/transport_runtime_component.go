package daemon

import (
	"context"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/telemetry"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
)

type TransportRuntime struct {
	runner        *AgentRuntime
	sensor        sensorruntime.Runtime
	batcher       *telemetry.Batcher
	sender        *telemetry.Sender
	startedAt     time.Time
	scopeType     string
	scopeSelector string
}

func NewTransportRuntime(runner *AgentRuntime, sensor sensorruntime.Runtime, source any, rest ...any) *TransportRuntime {
	batcher, sender, startedAt, scopeType, scopeSelector := transportArgs(runner, source, rest...)
	return &TransportRuntime{
		runner:        runner,
		sensor:        sensor,
		batcher:       batcher,
		sender:        sender,
		startedAt:     startedAt,
		scopeType:     scopeType,
		scopeSelector: scopeSelector,
	}
}

func transportArgs(runner *AgentRuntime, source any, rest ...any) (*telemetry.Batcher, *telemetry.Sender, time.Time, string, string) {
	var batcher *telemetry.Batcher
	var sender *telemetry.Sender
	var startedAt time.Time
	var scopeType, scopeSelector string
	if b, ok := source.(*telemetry.Batcher); ok {
		batcher = b
		if len(rest) > 0 {
			sender, _ = rest[0].(*telemetry.Sender)
		}
		if len(rest) > 1 {
			startedAt, _ = rest[1].(time.Time)
		}
		if len(rest) > 2 {
			scopeType, _ = rest[2].(string)
		}
		if len(rest) > 3 {
			scopeSelector, _ = rest[3].(string)
		}
	}
	if runner == nil {
		return telemetry.NewBatcher(nil, 0, 0, 0), &telemetry.Sender{Appender: localBatchAppender{}}, time.Now().UTC(), "", ""
	}
	if batcher == nil {
		batcher = telemetry.NewBatcher(runner.newDataBatch, runner.Config.Telemetry.BatchSize, runner.Config.Telemetry.FlushInterval, 64)
	}
	if sender == nil {
		sender = &telemetry.Sender{Appender: localBatchAppender{}, Batcher: batcher}
	}
	if sender.Batcher == nil {
		sender.Batcher = batcher
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	return batcher, sender, startedAt, scopeType, scopeSelector
}

func (r *TransportRuntime) RunDataFlow(ctx context.Context) {
	if r == nil || r.runner == nil {
		return
	}
	go r.batcher.Run(ctx)
	r.sender.Run(ctx)
}

func (r *TransportRuntime) RunControlFlow(ctx context.Context) {
	if r == nil || r.runner == nil || r.runner.Config.Manager.Transport != "grpc" {
		return
	}
	r.runControlFlow(ctx)
}
