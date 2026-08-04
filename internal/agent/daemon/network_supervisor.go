package daemon

import (
	"context"
	"reflect"
	"sync"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
)

type networkSupervisor struct {
	transitionMu    sync.Mutex
	mu              sync.Mutex
	parent          context.Context
	startStandalone func(context.Context)
	startManaged    func(context.Context, localstore.Enrollment)
	cancel          context.CancelFunc
	done            chan struct{}
	enrollment      localstore.Enrollment
}

func newNetworkSupervisor(parent context.Context, startStandalone func(context.Context), startManaged func(context.Context, localstore.Enrollment)) *networkSupervisor {
	return &networkSupervisor{parent: parent, startStandalone: startStandalone, startManaged: startManaged}
}

func (s *networkSupervisor) ApplyEnrollment(enrollment localstore.Enrollment) {
	if enrollment.State != localstore.StateManaged && enrollment.State != localstore.StateEnrolling && enrollment.State != localstore.StateUnenrolling {
		enrollment = localstore.Enrollment{State: localstore.StateStandalone}
	}
	s.transitionMu.Lock()
	defer s.transitionMu.Unlock()
	s.mu.Lock()
	if s.cancel != nil && reflect.DeepEqual(s.enrollment, enrollment) {
		s.mu.Unlock()
		return
	}
	cancel, done := s.detachLocked()
	s.mu.Unlock()
	stopNetworkFlow(cancel, done)
	ctx, cancel := context.WithCancel(s.parent)
	done = make(chan struct{})
	s.mu.Lock()
	s.cancel = cancel
	s.done = done
	s.enrollment = enrollment
	s.mu.Unlock()
	go s.run(ctx, enrollment, done)
}

func (s *networkSupervisor) Stop() {
	s.transitionMu.Lock()
	defer s.transitionMu.Unlock()
	s.mu.Lock()
	cancel, done := s.detachLocked()
	s.mu.Unlock()
	stopNetworkFlow(cancel, done)
}

func (s *networkSupervisor) PromoteEnrollment(enrollment localstore.Enrollment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil && s.enrollment.State == localstore.StateEnrolling && enrollment.State == localstore.StateManaged {
		s.enrollment = enrollment
	}
}

func (s *networkSupervisor) Managed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancel != nil && (s.enrollment.State == localstore.StateManaged || s.enrollment.State == localstore.StateEnrolling || s.enrollment.State == localstore.StateUnenrolling)
}

func (s *networkSupervisor) run(ctx context.Context, enrollment localstore.Enrollment, done chan struct{}) {
	defer close(done)
	if enrollment.State == localstore.StateStandalone {
		s.startStandalone(ctx)
		return
	}
	s.startManaged(ctx, enrollment)
}

func (s *networkSupervisor) detachLocked() (context.CancelFunc, chan struct{}) {
	cancel, done := s.cancel, s.done
	s.cancel = nil
	s.done = nil
	s.enrollment = localstore.Enrollment{}
	return cancel, done
}

func stopNetworkFlow(cancel context.CancelFunc, done <-chan struct{}) {
	if cancel == nil {
		return
	}
	cancel()
	if done != nil {
		<-done
	}
}
