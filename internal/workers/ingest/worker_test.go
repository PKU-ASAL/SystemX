package ingestworker

import (
	"context"
	"errors"
	"strings"
	"testing"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	platformkafka "github.com/sysarmor/sysarmor-next-project/internal/platform/kafka"
	platformopensearch "github.com/sysarmor/sysarmor-next-project/internal/platform/opensearch"
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
				Id:     "sig-worker",
				Name:   "payload_dropped",
				Labels: map[string]string{"scenario": "worker-scenario"},
				Where:  signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
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
	if got := st.ListSignals(store.LabelSelector{"scenario": "worker-scenario"}, "endpoint", false); len(got) != 1 {
		t.Fatalf("signals = %d, want 1", len(got))
	}
}

func TestProcessorIndexesDerivedDocumentsWithStableIDs(t *testing.T) {
	st := &store.Store{}
	indexer := &recordingIndexer{}
	processor := NewProcessor(st, indexer)
	scenario := "staged-recompute"
	labels := map[string]string{"scenario": scenario}

	mustProcess(t, processor, dataBatch("batch-drop", nil, []*signalv1.Signal{
		workerSignal("sig-drop", "payload_dropped", "lin-drop", labels, workerFile("/var/lib/app/plugins/helper")),
	}))
	mustProcess(t, processor, dataBatch("batch-connect", nil, []*signalv1.Signal{
		workerSignal("sig-connect", "suspicious_exec_connect", "lin-connect", labels, workerFile("/var/lib/app/plugins/helper"), workerSocket("10.66.0.99:443")),
	}))
	firstSignalID := lastDocIDContaining(indexer.docs, "sysarmor-signals", "dropped_payload_executed_and_connects")
	firstIncidentID := lastDocID(indexer.docs, "sysarmor-incidents")
	if firstSignalID == "" || firstIncidentID == "" {
		t.Fatalf("missing derived docs: %+v", indexer.docs)
	}

	mustProcess(t, processor, dataBatch("batch-noise", []*eventv1.CanonicalEvent{{
		Id:       "ev-noise",
		Labels:   labels,
		Behavior: "process.exec",
	}}, nil))
	if got := lastDocIDContaining(indexer.docs, "sysarmor-signals", "dropped_payload_executed_and_connects"); got != firstSignalID {
		t.Fatalf("cloud signal document id = %q, want stable %q", got, firstSignalID)
	}
	if got := lastDocID(indexer.docs, "sysarmor-incidents"); got != firstIncidentID {
		t.Fatalf("incident document id = %q, want stable %q", got, firstIncidentID)
	}
}

type recordingIndexer struct {
	docs []platformopensearch.Document
}

func (i *recordingIndexer) Index(_ context.Context, doc platformopensearch.Document) error {
	i.docs = append(i.docs, doc)
	return nil
}

func mustProcess(t *testing.T, processor *Processor, batch *dataplanev1.DataBatch) {
	t.Helper()
	if _, err := processor.Process(context.Background(), batch); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
}

func dataBatch(id string, events []*eventv1.CanonicalEvent, signals []*signalv1.Signal) *dataplanev1.DataBatch {
	batch := &dataplanev1.DataBatch{
		Header: &dataplanev1.BatchHeader{BatchId: id, TenantId: "default", AgentId: "agent-worker", HostId: "host-worker"},
	}
	for _, ev := range events {
		batch.Events = append(batch.Events, &dataplanev1.EventFrame{Event: ev})
	}
	for _, sig := range signals {
		batch.Signals = append(batch.Signals, &dataplanev1.SignalFrame{Signal: sig})
	}
	return batch
}

func workerSignal(id, name, lineage string, labels map[string]string, entities ...*signalv1.EntityRef) *signalv1.Signal {
	return &signalv1.Signal{
		Id:        id,
		Name:      name,
		Where:     signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		LineageId: lineage,
		Labels:    labels,
		Entities:  entities,
	}
}

func workerFile(path string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "file", Key: "file:" + path, Role: "object"}
}

func workerSocket(addr string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "socket", Key: "socket:" + addr, Role: "object"}
}

func lastDocID(docs []platformopensearch.Document, index string) string {
	for i := len(docs) - 1; i >= 0; i-- {
		if docs[i].Index == index {
			return docs[i].ID
		}
	}
	return ""
}

func lastDocIDContaining(docs []platformopensearch.Document, index, needle string) string {
	for i := len(docs) - 1; i >= 0; i-- {
		if docs[i].Index == index && strings.Contains(string(docs[i].Body), needle) {
			return docs[i].ID
		}
	}
	return ""
}
