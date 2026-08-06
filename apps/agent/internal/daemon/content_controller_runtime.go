package daemon

import (
	"fmt"
	"strings"

	agentcontent "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/content"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/detection"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func (r *AgentRuntime) applyContentUpdate(req *controlplanev1.ApplyContentRequest) *controlplanev1.ControlAck {
	if req == nil {
		return rejectedAck(r.Config, nil, "content", "content update request is required")
	}
	if err := r.validateControlContext(req.GetContext()); err != nil {
		return rejectedAck(r.Config, req.GetContext(), "content", err.Error())
	}
	var report detection.ApplyReport
	record, err := r.contentStore().Apply(req.GetContentJson(), req.GetAllowUnsigned(), true)
	if err != nil {
		return rejectedAck(r.Config, req.GetContext(), "content", err.Error())
	}
	status := record.Status
	if req.GetDryRun() {
		status = "validated"
	} else {
		var transactionAck *controlplanev1.ControlAck
		record, report, status, transactionAck = r.applyContentTransaction(req)
		if transactionAck != nil {
			return transactionAck
		}
	}
	message := fmt.Sprintf("content %s %s@%s digest=%s", status, record.Ref, record.Version, record.Digest)
	if len(report.Warnings) > 0 {
		message += "; detection dependencies degraded: " + strings.Join(report.Warnings, "; ")
	}
	return &controlplanev1.ControlAck{
		RequestId: requestID(req.GetContext()),
		TenantId:  r.Config.Agent.TenantID,
		AgentId:   r.Config.Agent.ID,
		Status:    status,
		Message:   message,
		PolicyId:  record.Ref,
		Sections: []*controlplanev1.AppliedSection{{
			Name:    "content",
			Status:  status,
			Message: message,
		}},
	}
}

func (r *AgentRuntime) applyContentTransaction(req *controlplanev1.ApplyContentRequest) (record agentcontent.Record, report detection.ApplyReport, status string, ack *controlplanev1.ControlAck) {
	r.withDetectionUpdateTransaction(func() {
		var snapshot agentcontent.Snapshot
		var err error
		record, snapshot, err = r.contentStore().Prepare(req.GetContentJson(), req.GetAllowUnsigned())
		if err != nil {
			ack = rejectedAck(r.Config, req.GetContext(), "content", err.Error())
			return
		}
		var engine *detection.Engine
		engine, report = r.buildDetectionWithSnapshot(snapshot)
		if report.Status == "rejected" {
			message := "content rejected; detection rebuild failed: " + strings.Join(report.Details, "; ")
			r.setDetectionStatus(r.activePolicy(), report, r.contentStore().Snapshot())
			ack = rejectedAck(r.Config, req.GetContext(), "content", message)
			return
		}
		if err = r.commitDetectionContent(record, snapshot, engine, report); err != nil {
			ack = rejectedAck(r.Config, req.GetContext(), "content", err.Error())
			return
		}
		status = record.Status
		if report.Status == "degraded" {
			status = "degraded"
		}
	})
	return record, report, status, ack
}
