package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
)

func TestNetworkSupervisorAppliesEnrollmentIdempotently(t *testing.T) {
	starts := make(chan struct{}, 3)
	supervisor := newNetworkSupervisor(context.Background(), func(ctx context.Context, enrollment localstore.Enrollment) {
		starts <- struct{}{}
		<-ctx.Done()
	})
	enrollment := localstore.Enrollment{State: localstore.StateManaged, AgentID: "a", GatewayAddress: "g"}
	supervisor.ApplyEnrollment(enrollment)
	waitStart(t, starts)
	supervisor.ApplyEnrollment(enrollment)
	select {
	case <-starts:
		t.Fatal("identical enrollment restarted network")
	case <-time.After(10 * time.Millisecond):
	}
	enrollment.GatewayAddress = "g2"
	supervisor.ApplyEnrollment(enrollment)
	waitStart(t, starts)
	supervisor.StopManaged()
	if supervisor.Managed() {
		t.Fatal("supervisor still managed")
	}
}

func waitStart(t *testing.T, starts <-chan struct{}) {
	t.Helper()
	select {
	case <-starts:
	case <-time.After(time.Second):
		t.Fatal("network did not start")
	}
}
