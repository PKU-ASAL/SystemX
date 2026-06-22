package ingestworker

import (
	"context"
	"errors"
	"testing"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	platformkafka "github.com/sysarmor/sysarmor-next-project/internal/platform/kafka"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"google.golang.org/protobuf/encoding/protojson"
)

type stubConsumer struct {
	messages  []platformkafka.Message
	committed int
}

func (c *stubConsumer) Fetch(context.Context) (platformkafka.Message, error) {
	if len(c.messages) == 0 {
		return platformkafka.Message{}, context.Canceled
	}
	msg := c.messages[0]
	c.messages = c.messages[1:]
	return msg, nil
}

func (c *stubConsumer) Commit(context.Context, platformkafka.Message) error {
	c.committed++
	return nil
}

func (c *stubConsumer) Close() error { return nil }

func TestWorkerConsumesKafkaUploadAndProcessesAfterCommit(t *testing.T) {
	raw, err := protojson.Marshal(&dataplanev1.DataBatch{
		Header: &dataplanev1.BatchHeader{BatchId: "batch-worker", TenantId: "default", AgentId: "agent-worker", HostId: "host-worker"},
		Signals: []*dataplanev1.SignalFrame{{
			Signal: &signalv1.Signal{
				Id:       "sig-worker",
				Name:     "payload_dropped",
				Scenario: "worker-scenario",
				Where:    signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	consumer := &stubConsumer{messages: []platformkafka.Message{{Key: "default:agent-worker:batch-worker", Value: raw}}}
	st := &store.Store{}
	err = NewWorker(consumer, NewProcessor(st, nil)).Run(context.Background())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled after first message", err)
	}
	if consumer.committed != 1 {
		t.Fatalf("committed = %d, want 1", consumer.committed)
	}
	if got := st.ListSignals("worker-scenario", "endpoint", false); len(got) != 1 {
		t.Fatalf("signals = %d, want 1", len(got))
	}
}
