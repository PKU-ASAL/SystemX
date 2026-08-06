package localapi

import (
	agentcontent "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/content"
	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func policyCommand(req *controlplanev1.ApplyPolicyRequest) agentcontrol.PolicyCommand {
	return agentcontrol.PolicyCommand{
		Context: controlRequestContext(req.GetContext()), PolicyType: req.GetPolicyType(),
		Document: req.GetPolicyJson(), DryRun: req.GetDryRun(), Source: agentcontrol.PolicySourceStandalone,
	}
}

func contentCommand(req *controlplanev1.ApplyContentRequest) agentcontrol.ContentCommand {
	return agentcontrol.ContentCommand{
		Context: controlRequestContext(req.GetContext()), Document: req.GetContentJson(),
		DryRun: req.GetDryRun(), AllowUnsigned: req.GetAllowUnsigned(), Source: agentcontrol.PolicySourceStandalone,
	}
}

func enrollmentCommand(req *controlplanev1.EnrollRequest) agentcontrol.EnrollmentCommand {
	return agentcontrol.EnrollmentCommand{
		Context: controlRequestContext(req.GetContext()), ManagerURL: req.GetManagerUrl(),
		Token: req.GetEnrollmentToken(), UploadHistory: req.GetUploadHistory(),
	}
}

func unenrollmentCommand(req *controlplanev1.UnenrollRequest) agentcontrol.UnenrollmentCommand {
	return agentcontrol.UnenrollmentCommand{Context: controlRequestContext(req.GetContext())}
}

func controlRequestContext(req *controlplanev1.RequestContext) agentcontrol.RequestContext {
	return agentcontrol.RequestContext{RequestID: req.GetRequestId(), TenantID: req.GetTenantId(), AgentID: req.GetAgentId()}
}

func controlAck(result agentcontrol.Result) *controlplanev1.ControlAck {
	ack := &controlplanev1.ControlAck{
		RequestId: result.RequestID, TenantId: result.TenantID, AgentId: result.AgentID,
		Status: result.Status, Message: result.Message, PolicyId: result.PolicyID,
		PolicyVersion: result.Version, Details: append([]string(nil), result.Details...), ReportJson: result.ReportJSON,
	}
	for _, section := range result.Sections {
		ack.Sections = append(ack.Sections, &controlplanev1.AppliedSection{
			Name: section.Name, Status: section.Status, Message: section.Message,
			RequiresRestart: section.RequiresRestart, ReportJson: section.ReportJSON,
		})
	}
	return ack
}

func contentRecordMessage(record agentcontent.Record) *controlplanev1.ContentRecord {
	return &controlplanev1.ContentRecord{
		Ref: record.Ref, Kind: record.Kind, Version: record.Version, Digest: record.Digest,
		Signed: record.Signed, Status: record.Status, RawJson: record.RawJSON,
	}
}
