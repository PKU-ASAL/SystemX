package daemon

import (
	"fmt"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

type AgentSpool struct {
	queue *spool.Queue
}

func NewAgentSpool(queue *spool.Queue) *AgentSpool {
	return &AgentSpool{queue: queue}
}

func (s *AgentSpool) AppendBatch(batch *dataplanev1.DataBatch) (string, error) {
	if s == nil || s.queue == nil {
		return "", fmt.Errorf("agent spool is nil")
	}
	return s.queue.AppendDataBatch(batch)
}

func (s *AgentSpool) AppendEndpointEvent(runtime *EndpointRuntime, ev contract.EventEnvelope) (string, error) {
	if runtime == nil {
		return "", fmt.Errorf("endpoint runtime is nil")
	}
	batch, err := runtime.ProcessEvent(ev)
	if err != nil {
		return "", err
	}
	return s.AppendBatch(batch)
}

func (s *AgentSpool) AppendEndpointSignals(runtime *EndpointRuntime, signals []*signalv1.Signal) (string, error) {
	if runtime == nil {
		return "", fmt.Errorf("endpoint runtime is nil")
	}
	batch, err := runtime.ProcessSignals(signals)
	if err != nil {
		return "", err
	}
	return s.AppendBatch(batch)
}

func (s *AgentSpool) AckThrough(cursor string) error {
	if s == nil || s.queue == nil || cursor == "" {
		return nil
	}
	return s.queue.AckThrough(cursor)
}

func (s *AgentSpool) Queue() *spool.Queue {
	if s == nil {
		return nil
	}
	return s.queue
}
