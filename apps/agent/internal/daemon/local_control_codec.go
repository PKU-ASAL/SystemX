package daemon

import (
	agentcontent "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/content"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
)

func contentRecordMessage(record agentcontent.Record) *controlplanev1.ContentRecord {
	return &controlplanev1.ContentRecord{
		Ref:     record.Ref,
		Kind:    record.Kind,
		Version: record.Version,
		Digest:  record.Digest,
		Signed:  record.Signed,
		Status:  record.Status,
		RawJson: record.RawJSON,
	}
}
