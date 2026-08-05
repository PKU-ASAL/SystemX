package daemon

import (
	"context"
	"fmt"
	"io"

	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/dataappend"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/dataplane/v1"
)

type ExportResult struct {
	Committed bool
}

type Exporter interface {
	Export(context.Context, *dataplanev1.DataBatch) (ExportResult, error)
	Close() error
}

type cloudExporter struct {
	sender dataappend.BatchSender
}

func (e *cloudExporter) Export(_ context.Context, batch *dataplanev1.DataBatch) (ExportResult, error) {
	if e == nil || e.sender == nil {
		return ExportResult{}, fmt.Errorf("cloud exporter is not configured")
	}
	ack, err := e.sender.SendBatch(batch)
	if err != nil {
		return ExportResult{}, err
	}
	if !dataappend.AckCommitted(ack) {
		return ExportResult{}, fmt.Errorf("cloud export rejected: %s", ack.GetMessage())
	}
	return ExportResult{Committed: true}, nil
}

func (e *cloudExporter) Close() error {
	if closer, ok := e.sender.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
