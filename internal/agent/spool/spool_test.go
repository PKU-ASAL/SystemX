package spool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
)

func TestQueueAppendLoadAck(t *testing.T) {
	q, err := Open(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	id, err := q.AppendDataBatch(batch("event-1"))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if id != "00000000000000000001" {
		t.Fatalf("id = %q", id)
	}
	entries, err := q.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].ID != id {
		t.Fatalf("entries = %+v", entries)
	}
	loaded, err := q.LoadDataBatch(id)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.GetHeader().GetBatchId() != id {
		t.Fatalf("batch_id = %q, want %q", loaded.GetHeader().GetBatchId(), id)
	}
	if loaded.GetEvents()[0].GetEvent().GetId() != "event-1" {
		t.Fatalf("loaded batch = %+v", loaded)
	}
	if err := q.Ack(id); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	entries, err = q.List()
	if err != nil {
		t.Fatalf("List() after ack error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries after ack = %+v", entries)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(entriesPath(t, q)), "cursor.json")); err != nil {
		t.Fatalf("cursor not written: %v", err)
	}
}

func TestQueueReloadsExistingBatchesInOrder(t *testing.T) {
	dir := t.TempDir()
	q, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.AppendDataBatch(batch("event-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.AppendDataBatch(batch("event-2")); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := reopened.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != "00000000000000000001" || entries[1].ID != "00000000000000000002" {
		t.Fatalf("entries = %+v", entries)
	}
	stats, err := reopened.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.QueuedBatches != 2 || stats.QueuedBytes == 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestQueueAckThroughRemovesAckedPrefix(t *testing.T) {
	q, err := Open(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"event-1", "event-2", "event-3"} {
		if _, err := q.AppendDataBatch(batch(id)); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.AckThrough("00000000000000000002"); err != nil {
		t.Fatal(err)
	}
	entries, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "00000000000000000003" {
		t.Fatalf("entries after AckThrough = %+v", entries)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(entriesPath(t, q)), "cursor.json")); err != nil {
		t.Fatalf("cursor not written: %v", err)
	}
}

func TestQueueListSkipsPersistedAckCursor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spool")
	q, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := q.AppendDataBatch(batch("event-1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.AppendDataBatch(batch("event-2")); err != nil {
		t.Fatal(err)
	}
	if err := q.writeCursorLocked(first); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := reopened.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "00000000000000000002" {
		t.Fatalf("entries after persisted cursor = %+v", entries)
	}
	next, err := reopened.AppendDataBatch(batch("event-3"))
	if err != nil {
		t.Fatal(err)
	}
	if next != "00000000000000000003" {
		t.Fatalf("next id = %q, want 00000000000000000003", next)
	}
}

func TestQueueQuarantinesCorruptBatch(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spool")
	q, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	id, err := q.AppendDataBatch(batch("event-1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(q.batchPath(id), []byte("{not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := q.LoadDataBatch(id); err == nil {
		t.Fatal("LoadDataBatch() error = nil")
	}
	if _, err := os.Stat(filepath.Join(dir, id+".batch.json.corrupt")); err != nil {
		t.Fatalf("corrupt batch not quarantined: %v", err)
	}
	stats, err := q.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.CorruptBatches != 1 || stats.DroppedBatches != 1 || stats.LastError == "" {
		t.Fatalf("stats after corrupt quarantine = %+v", stats)
	}
	entries, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries after corrupt quarantine = %+v", entries)
	}
}

func TestQueueBackpressureDropsWhenOverLimit(t *testing.T) {
	q, err := OpenWithLimit(filepath.Join(t.TempDir(), "spool"), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = q.AppendDataBatch(batch("event-1"))
	if err == nil {
		t.Fatal("Append() error = nil")
	}
	if !IsBackpressure(err) {
		t.Fatalf("Append() error = %v, want backpressure", err)
	}
	stats, err := q.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.BackpressureCount != 1 || stats.DroppedBatches != 1 || stats.DroppedBytes == 0 || stats.LastError == "" {
		t.Fatalf("stats = %+v", stats)
	}
	entries, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestQueueLimitAllowsBatchWithinCapacity(t *testing.T) {
	q, err := OpenWithLimit(filepath.Join(t.TempDir(), "spool"), 4096)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.AppendDataBatch(batch("event-1")); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	stats, err := q.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.MaxBytes != 4096 || stats.QueuedBatches != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestQueueDataBatchWatchAfterID(t *testing.T) {
	q, err := Open(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := q.AppendDataBatch(dataBatch("event-1"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entries, err := q.Watch(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := q.AppendDataBatch(dataBatch("event-2"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case entry := <-entries:
		if entry.ID != second {
			t.Fatalf("watch entry ID = %q, want %q", entry.ID, second)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watch entry")
	}
	loaded, err := q.LoadDataBatch(second)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GetHeader().GetBatchId() != second || loaded.GetEvents()[0].GetEvent().GetId() != "event-2" {
		t.Fatalf("loaded data batch = %+v", loaded)
	}
}

func batch(id string) *dataplanev1.DataBatch {
	return &dataplanev1.DataBatch{
		Header: &dataplanev1.BatchHeader{AgentId: "agent-a", HostId: "host-a", TenantId: "default"},
		Events: []*dataplanev1.EventFrame{{
			Sequence: 1,
			Event: &eventv1.CanonicalEvent{
				Id:       id,
				AgentId:  "agent-a",
				HostId:   "host-a",
				Behavior: "process.exec",
			},
		}},
	}
}

func dataBatch(id string) *dataplanev1.DataBatch { return batch(id) }

func entriesPath(t *testing.T, q *Queue) string {
	t.Helper()
	entries, err := q.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 0 {
		return entries[0].Path
	}
	return filepath.Join(q.dir, "empty")
}
