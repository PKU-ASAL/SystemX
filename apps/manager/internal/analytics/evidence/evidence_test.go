package evidence

import (
	"testing"

	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
)

func TestFromSignalsBuildsDeduplicatedNodes(t *testing.T) {
	got := FromSignals([]*signalv1.Signal{{
		Entities: []*signalv1.EntityRef{
			{Kind: "file", Key: "/dev/shm/x.sh", Role: "object"},
			{Kind: "file", Key: "file:/dev/shm/x.sh", Role: "object"},
			{Kind: "socket", Key: "10.66.0.99:443", Role: "object"},
		},
	}})
	if len(got.GetNodes()) != 2 {
		t.Fatalf("nodes = %d, want 2: %#v", len(got.GetNodes()), got.GetNodes())
	}
	if got.GetNodes()[0].GetId() != "file:/dev/shm/x.sh" {
		t.Fatalf("first node id = %q", got.GetNodes()[0].GetId())
	}
}
