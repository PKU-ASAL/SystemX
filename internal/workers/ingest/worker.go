package ingestworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	platformkafka "github.com/sysarmor/sysarmor-next-project/internal/platform/kafka"
	platformopensearch "github.com/sysarmor/sysarmor-next-project/internal/platform/opensearch"
	"google.golang.org/protobuf/encoding/protojson"
)

type Worker struct {
	consumer  platformkafka.Consumer
	processor *Processor
	dlq       platformkafka.Producer
}

func NewWorker(consumer platformkafka.Consumer, processor *Processor) *Worker {
	return &Worker{consumer: consumer, processor: processor}
}

func NewWorkerWithDLQ(consumer platformkafka.Consumer, processor *Processor, dlq platformkafka.Producer) *Worker {
	return &Worker{consumer: consumer, processor: processor, dlq: dlq}
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
			if err := w.reject(ctx, msg, "invalid_data_batch", err); err != nil {
				return err
			}
			continue
		}
		if err := validateBatchIdentity(batch); err != nil {
			if err := w.reject(ctx, msg, "invalid_data_batch", err); err != nil {
				return err
			}
			continue
		}
		stabilizeBatchTime(batch, time.Now().UTC())
		if err := w.processWithRetry(ctx, batch); err != nil {
			if platformopensearch.ErrorClassOf(err) == platformopensearch.ErrorPermanent {
				if rejectErr := w.reject(ctx, msg, "permanent_projection", err); rejectErr != nil {
					return rejectErr
				}
				continue
			}
			return fmt.Errorf("process raw data batch key=%q: %w", msg.Key, err)
		}
		if err := w.consumer.Commit(ctx, msg); err != nil {
			return fmt.Errorf("commit raw data batch key=%q: %w", msg.Key, err)
		}
	}
}

func stabilizeBatchTime(batch *dataplanev1.DataBatch, fallback time.Time) {
	latest := batch.GetHeader().GetCreatedAtUnixNano()
	for _, frame := range batch.GetEvents() {
		latest = maxUnixNano(latest, frame.GetObservedAt(), frame.GetEvent().GetOccurredAtNs())
	}
	for _, frame := range batch.GetSignals() {
		latest = maxUnixNano(latest, frame.GetObservedAt(), 0)
	}
	if latest <= 0 {
		latest = fallback.UnixNano()
	}
	batch.Header.CreatedAtUnixNano = latest
}

func maxUnixNano(current int64, observedAt string, occurredAt uint64) int64 {
	if parsed, err := time.Parse(time.RFC3339Nano, observedAt); err == nil && parsed.UnixNano() > current {
		current = parsed.UnixNano()
	}
	if occurredAt > 0 && int64(occurredAt) > current {
		current = int64(occurredAt)
	}
	return current
}

func validateBatchIdentity(batch *dataplanev1.DataBatch) error {
	if batch.GetHeader() == nil {
		return fmt.Errorf("data batch header is required")
	}
	if batch.GetHeader().GetBatchId() == "" || batch.GetHeader().GetTenantId() == "" || batch.GetHeader().GetAgentId() == "" {
		return fmt.Errorf("batch_id, tenant_id, and agent_id are required")
	}
	return nil
}

func (w *Worker) processWithRetry(ctx context.Context, batch *dataplanev1.DataBatch) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := w.processor.Process(ctx, batch); err == nil {
			return nil
		} else {
			lastErr = err
			if platformopensearch.ErrorClassOf(err) == platformopensearch.ErrorPermanent {
				return err
			}
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(10*(1<<attempt)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

type deadLetterEnvelope struct {
	SourceTopic     string    `json:"source_topic"`
	SourcePartition int       `json:"source_partition"`
	SourceOffset    int64     `json:"source_offset"`
	SourceKey       string    `json:"source_key"`
	FailureClass    string    `json:"failure_class"`
	FailureMessage  string    `json:"failure_message"`
	FailureCode     string    `json:"failure_code"`
	DocumentIndex   string    `json:"document_index,omitempty"`
	DocumentID      string    `json:"document_id,omitempty"`
	ObservedAt      time.Time `json:"observed_at"`
	Payload         []byte    `json:"payload"`
}

func (w *Worker) reject(ctx context.Context, msg platformkafka.Message, class string, cause error) error {
	if w.dlq == nil {
		return fmt.Errorf("decode raw data batch key=%q: %w", msg.Key, cause)
	}
	envelope := deadLetterEnvelope{
		SourceTopic: msg.Topic, SourcePartition: msg.Partition, SourceOffset: msg.Offset,
		SourceKey: msg.Key, FailureClass: class, FailureMessage: cause.Error(),
		FailureCode: class, ObservedAt: time.Now().UTC(), Payload: msg.Value,
	}
	var projection *platformopensearch.ProjectionError
	if errors.As(cause, &projection) {
		envelope.FailureCode = fmt.Sprintf("opensearch_%d", projection.Status)
		envelope.DocumentIndex = projection.Index
		envelope.DocumentID = projection.ID
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encode dead letter key=%q: %w", msg.Key, err)
	}
	if err := w.dlq.Append(ctx, platformkafka.Message{Topic: msg.Topic + ".dlq", Key: msg.Key, Value: body}); err != nil {
		return fmt.Errorf("publish dead letter key=%q: %w", msg.Key, err)
	}
	if err := w.consumer.Commit(ctx, msg); err != nil {
		return fmt.Errorf("commit rejected data batch key=%q: %w", msg.Key, err)
	}
	return nil
}
