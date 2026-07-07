package daemon

import (
	"fmt"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/normalize"
	"github.com/sysarmor/sysarmor-next-project/internal/sensors/contract"
)

type EndpointRuntime struct {
	runner     *AgentRuntime
	normalizer *normalize.Normalizer
}

func NewEndpointRuntime(runner *AgentRuntime, normalizer *normalize.Normalizer) *EndpointRuntime {
	return &EndpointRuntime{runner: runner, normalizer: normalizer}
}

func (r *EndpointRuntime) ProcessEvent(ev contract.EventEnvelope) (*dataplanev1.DataBatch, error) {
	if r == nil || r.runner == nil || r.normalizer == nil {
		return nil, fmt.Errorf("endpoint runtime is not initialized")
	}
	if ev.SensorEvent == nil {
		return nil, fmt.Errorf("sensor event is nil")
	}
	if ev.SensorEvent.RawRef == "" {
		ev.SensorEvent.RawRef = ev.RawRef
	}
	canonical := r.normalizer.Normalize(ev.SensorEvent)
	canonical.Labels = mergeLabels(canonical.GetLabels(), r.runner.policyLabels())
	signals := r.runner.currentDetection().Process(canonical)
	return r.runner.dataBatchForEvent(canonical, signals), nil
}

func (r *EndpointRuntime) ProcessSignals(signals []*signalv1.Signal) (*dataplanev1.DataBatch, error) {
	if r == nil || r.runner == nil {
		return nil, fmt.Errorf("endpoint runtime is not initialized")
	}
	return r.runner.dataBatchForSignals(signals), nil
}
