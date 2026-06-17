package daemon

import (
	"sync"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

const defaultLocalStreamCapacity = 8192

type localStreamBuffer struct {
	mu            sync.RWMutex
	nextEventSeq  uint64
	nextSignalSeq uint64
	eventCap      int
	signalCap     int
	events        []*controlv1.EventFrame
	eventByID     map[string]*controlv1.EventFrame
	signals       []*controlv1.SignalFrame
	eventSubs     map[chan *controlv1.EventFrame]struct{}
	signalSubs    map[chan *controlv1.SignalFrame]struct{}
}

func newLocalStreamBuffer(capacity int) *localStreamBuffer {
	if capacity <= 0 {
		capacity = defaultLocalStreamCapacity
	}
	return &localStreamBuffer{
		eventCap:   capacity,
		signalCap:  capacity,
		eventByID:  make(map[string]*controlv1.EventFrame),
		eventSubs:  make(map[chan *controlv1.EventFrame]struct{}),
		signalSubs: make(map[chan *controlv1.SignalFrame]struct{}),
	}
}

func (b *localStreamBuffer) publishEvent(tenantID, agentID string, event *eventv1.CanonicalEvent) {
	if b == nil || event == nil {
		return
	}
	b.mu.Lock()
	b.nextEventSeq++
	frame := &controlv1.EventFrame{
		TenantId:   tenantID,
		AgentId:    agentID,
		Sequence:   b.nextEventSeq,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Event:      event,
	}
	b.events = appendEventBounded(b.events, frame, b.eventCap, b.eventByID)
	subs := make([]chan *controlv1.EventFrame, 0, len(b.eventSubs))
	for ch := range b.eventSubs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- frame:
		default:
		}
	}
}

func (b *localStreamBuffer) publishSignals(tenantID, agentID string, signals []*signalv1.Signal) {
	if b == nil || len(signals) == 0 {
		return
	}
	for _, sig := range signals {
		b.publishSignal(tenantID, agentID, sig)
	}
}

func (b *localStreamBuffer) publishSignal(tenantID, agentID string, signal *signalv1.Signal) {
	if b == nil || signal == nil {
		return
	}
	b.mu.Lock()
	b.nextSignalSeq++
	frame := &controlv1.SignalFrame{
		TenantId:   tenantID,
		AgentId:    agentID,
		Sequence:   b.nextSignalSeq,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Signal:     signal,
	}
	b.signals = appendBounded(b.signals, frame, b.signalCap)
	subs := make([]chan *controlv1.SignalFrame, 0, len(b.signalSubs))
	for ch := range b.signalSubs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- frame:
		default:
		}
	}
}

func (b *localStreamBuffer) recentEvents() []*controlv1.EventFrame {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]*controlv1.EventFrame(nil), b.events...)
}

func (b *localStreamBuffer) getEvent(id string) (*controlv1.EventFrame, bool) {
	if b == nil || id == "" {
		return nil, false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	frame, ok := b.eventByID[id]
	return frame, ok
}

func (b *localStreamBuffer) recentSignals() []*controlv1.SignalFrame {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]*controlv1.SignalFrame(nil), b.signals...)
}

func (b *localStreamBuffer) subscribeEvents() (<-chan *controlv1.EventFrame, func()) {
	ch := make(chan *controlv1.EventFrame, 64)
	if b == nil {
		close(ch)
		return ch, func() {}
	}
	b.mu.Lock()
	b.eventSubs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.eventSubs, ch)
		close(ch)
		b.mu.Unlock()
	}
}

func (b *localStreamBuffer) subscribeSignals() (<-chan *controlv1.SignalFrame, func()) {
	ch := make(chan *controlv1.SignalFrame, 64)
	if b == nil {
		close(ch)
		return ch, func() {}
	}
	b.mu.Lock()
	b.signalSubs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.signalSubs, ch)
		close(ch)
		b.mu.Unlock()
	}
}

func appendBounded[T any](in []T, item T, cap int) []T {
	if cap <= 0 {
		cap = defaultLocalStreamCapacity
	}
	in = append(in, item)
	if len(in) <= cap {
		return in
	}
	out := make([]T, cap)
	copy(out, in[len(in)-cap:])
	return out
}

func appendEventBounded(in []*controlv1.EventFrame, item *controlv1.EventFrame, cap int, byID map[string]*controlv1.EventFrame) []*controlv1.EventFrame {
	if byID != nil && item.GetEvent().GetId() != "" {
		byID[item.GetEvent().GetId()] = item
	}
	out := appendBounded(in, item, cap)
	if len(out) == len(in)+1 {
		return out
	}
	if byID != nil {
		keep := map[string]bool{}
		for _, frame := range out {
			if id := frame.GetEvent().GetId(); id != "" {
				keep[id] = true
			}
		}
		for id := range byID {
			if !keep[id] {
				delete(byID, id)
			}
		}
	}
	return out
}
