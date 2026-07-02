package gateway

import "sync/atomic"

type Metrics struct {
	acceptedBatches  atomic.Uint64
	duplicateBatches atomic.Uint64
	rejectedBatches  atomic.Uint64
	handoffErrors    atomic.Uint64
	acceptedEvents   atomic.Uint64
	acceptedSignals  atomic.Uint64
}

type MetricsSnapshot struct {
	AcceptedBatches  uint64 `json:"accepted_batches"`
	DuplicateBatches uint64 `json:"duplicate_batches"`
	RejectedBatches  uint64 `json:"rejected_batches"`
	HandoffErrors    uint64 `json:"handoff_errors"`
	AcceptedEvents   uint64 `json:"accepted_events"`
	AcceptedSignals  uint64 `json:"accepted_signals"`
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	return MetricsSnapshot{
		AcceptedBatches:  m.acceptedBatches.Load(),
		DuplicateBatches: m.duplicateBatches.Load(),
		RejectedBatches:  m.rejectedBatches.Load(),
		HandoffErrors:    m.handoffErrors.Load(),
		AcceptedEvents:   m.acceptedEvents.Load(),
		AcceptedSignals:  m.acceptedSignals.Load(),
	}
}
