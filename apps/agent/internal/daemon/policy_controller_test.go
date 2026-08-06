package daemon

import (
	"testing"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func TestControlResultPreservesProtocolFields(t *testing.T) {
	want := &controlplanev1.ControlAck{
		RequestId: "request-a", TenantId: "tenant-a", AgentId: "agent-a",
		Status: "degraded", Message: "applied with warnings", PolicyId: "policy-a", PolicyVersion: 7,
		Details: []string{"warning-a"}, ReportJson: `{"status":"degraded"}`,
		Sections: []*controlplanev1.AppliedSection{{
			Name: "collection", Status: "applied", Message: "updated",
			RequiresRestart: true, ReportJson: `{"backend":"tetragon"}`,
		}},
	}

	got := controlResult(want)
	if got.RequestID != want.GetRequestId() || got.TenantID != want.GetTenantId() || got.AgentID != want.GetAgentId() ||
		got.Status != want.GetStatus() || got.Message != want.GetMessage() || got.PolicyID != want.GetPolicyId() ||
		got.Version != want.GetPolicyVersion() || got.ReportJSON != want.GetReportJson() || len(got.Details) != 1 ||
		len(got.Sections) != 1 || !got.RequiresRestart || got.Sections[0].ReportJSON != want.GetSections()[0].GetReportJson() {
		t.Fatalf("controlResult()=%+v want=%+v", got, want)
	}
}
