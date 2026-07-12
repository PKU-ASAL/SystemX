package daemon

import (
	"context"
	"reflect"
	"sync"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
)

type networkSupervisor struct {
	mu         sync.Mutex
	parent     context.Context
	start      func(context.Context, localstore.Enrollment)
	cancel     context.CancelFunc
	enrollment localstore.Enrollment
}

func newNetworkSupervisor(parent context.Context, start func(context.Context, localstore.Enrollment)) *networkSupervisor {
	return &networkSupervisor{parent: parent, start: start}
}

func (s *networkSupervisor) ApplyEnrollment(enrollment localstore.Enrollment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if enrollment.State != localstore.StateManaged {
		s.stopLocked()
		return
	}
	if s.cancel != nil && reflect.DeepEqual(s.enrollment, enrollment) {
		return
	}
	s.stopLocked()
	ctx, cancel := context.WithCancel(s.parent)
	s.cancel = cancel
	s.enrollment = enrollment
	go s.start(ctx, enrollment)
}

func (s *networkSupervisor) StopManaged() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *networkSupervisor) Managed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancel != nil
}

func (s *networkSupervisor) stopLocked() {
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = nil
	s.enrollment = localstore.Enrollment{}
}
