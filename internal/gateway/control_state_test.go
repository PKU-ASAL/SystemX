package gateway

import (
	"fmt"
	"testing"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
)

func TestControlConnectionStateBoundsReplayCache(t *testing.T) {
	state := controlConnectionState{repliesByRequestID: map[string][]*controlplanev1.ControlFrame{}}
	for i := 0; i < maxControlReplayCache+1; i++ {
		requestID := fmt.Sprintf("req-%d", i)
		state.remember(requestID, []*controlplanev1.ControlFrame{{RequestId: requestID}})
	}

	if _, ok := state.replay("req-0"); ok {
		t.Fatal("oldest replay entry was retained after cache exceeded capacity")
	}
	if _, ok := state.replay(fmt.Sprintf("req-%d", maxControlReplayCache)); !ok {
		t.Fatal("newest replay entry was not retained")
	}
	if len(state.repliesByRequestID) > maxControlReplayCache {
		t.Fatalf("replay cache size = %d, want <= %d", len(state.repliesByRequestID), maxControlReplayCache)
	}
}
