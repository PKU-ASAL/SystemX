package ingestworker

import (
	"context"
	"fmt"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	platformkafka "github.com/sysarmor/sysarmor-next-project/internal/platform/kafka"
	"google.golang.org/protobuf/encoding/protojson"
)

type Worker struct {
	consumer  platformkafka.Consumer
	processor *Processor
}

func NewWorker(consumer platformkafka.Consumer, processor *Processor) *Worker {
	return &Worker{consumer: consumer, processor: processor}
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || w.consumer == nil || w.processor == nil {
		return fmt.Errorf("ingest worker requires consumer and processor")
	}
	for {
		msg, err := w.consumer.Fetch(ctx)
		if err != nil {
			return err
		}
		batch := &dataplanev1.DataBatch{}
		if err := protojson.Unmarshal(msg.Value, batch); err != nil {
			return fmt.Errorf("decode raw data batch key=%q: %w", msg.Key, err)
		}
		if _, err := w.processor.Process(ctx, batch); err != nil {
			return fmt.Errorf("process raw data batch key=%q: %w", msg.Key, err)
		}
		if err := w.consumer.Commit(ctx, msg); err != nil {
			return fmt.Errorf("commit raw data batch key=%q: %w", msg.Key, err)
		}
	}
}
