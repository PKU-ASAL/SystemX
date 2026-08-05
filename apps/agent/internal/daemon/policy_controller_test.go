package daemon

import (
	"testing"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	"google.golang.org/protobuf/proto"
)

func TestControlAckRoundTripPreservesProtocolFields(t *testing.T) {
	want := &controlplanev1.ControlAck{
		RequestId: "request-a", TenantId: "tenant-a", AgentId: "agent-a",
		Status: "degraded", Message: "applied with warnings", PolicyId: "policy-a", PolicyVersion: 7,
		Details: []string{"warning-a"}, ReportJson: `{"status":"degraded"}`,
		Sections: []*controlplanev1.AppliedSection{{
			Name: "collection", Status: "applied", Message: "updated",
			RequiresRestart: true, ReportJson: `{"backend":"tetragon"}`,
		}},
	}

	got := controlAck(controlResult(want))
	if !proto.Equal(got, want) {
		t.Fatalf("round trip ack mismatch\n got: %+v\nwant: %+v", got, want)
	}
}
