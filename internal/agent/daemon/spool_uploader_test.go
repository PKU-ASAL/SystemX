package daemon

import (
	"context"
	"path/filepath"
	"testing"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
)

func TestSpoolUploaderCheckpointsOnlyCommittedAck(t *testing.T) {
	store, err := localstore.Open(t.Context(), localstore.Options{RootDir: filepath.Join(t.TempDir(), "state")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for sequence := uint64(1); sequence <= 2; sequence++ {
		if _, err := store.AppendBatch(t.Context(), &dataplanev1.DataBatch{Header: &dataplanev1.BatchHeader{BatchId: string(rune('a' + sequence - 1)), EventSeqStart: sequence}}); err != nil {
			t.Fatal(err)
		}
	}
	sender := &sequenceSender{acks: []*dataplanev1.DataAck{
		{Status: dataplanev1.DataAck_STATUS_ACCEPTED, Accepted: true},
		{Status: dataplanev1.DataAck_STATUS_RETRYABLE},
	}}
	uploader := &spoolUploader{store: store, sender: sender}
	if err := uploader.uploadAvailable(t.Context()); err == nil {
		t.Fatal("retryable ack returned nil")
	}
	checkpoint, err := store.Checkpoint(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.LastBatchID != "a" || len(sender.sent) != 2 {
		t.Fatalf("checkpoint=%+v sent=%v", checkpoint, sender.sent)
	}
}

func TestSpoolUploaderReplaysCheckpointBatchAfterRestart(t *testing.T) {
	store, err := localstore.Open(t.Context(), localstore.Options{RootDir: filepath.Join(t.TempDir(), "state")})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	position, err := store.AppendBatch(t.Context(), &dataplanev1.DataBatch{Header: &dataplanev1.BatchHeader{BatchId: "a", EventSeqStart: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCheckpoint(t.Context(), localstore.Checkpoint{SegmentID: position.SegmentID, RecordOffset: position.RecordOffset, LastBatchID: "a"}); err != nil {
		t.Fatal(err)
	}
	sender := &sequenceSender{acks: []*dataplanev1.DataAck{{Status: dataplanev1.DataAck_STATUS_DUPLICATE, Accepted: true}}}
	if err := (&spoolUploader{store: store, sender: sender}).uploadAvailable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 || sender.sent[0] != "a" {
		t.Fatalf("sent=%v", sender.sent)
	}
}

type sequenceSender struct {
	acks []*dataplanev1.DataAck
	sent []string
}

func (s *sequenceSender) SendBatch(batch *dataplanev1.DataBatch) (*dataplanev1.DataAck, error) {
	s.sent = append(s.sent, batch.GetHeader().GetBatchId())
	if len(s.acks) == 0 {
		return &dataplanev1.DataAck{Status: dataplanev1.DataAck_STATUS_ACCEPTED, Accepted: true}, nil
	}
	ack := s.acks[0]
	s.acks = s.acks[1:]
	return ack, nil
}
