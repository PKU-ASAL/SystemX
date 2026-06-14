package spool

import (
	"os"
	"path/filepath"
	"testing"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
)

func TestQueueAppendLoadAck(t *testing.T) {
	q, err := Open(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	id, err := q.Append(batch("event-1"))
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
	loaded, err := q.Load(id)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.GetEvents()[0].GetId() != "event-1" {
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
	if _, err := q.Append(batch("event-1")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Append(batch("event-2")); err != nil {
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

func batch(id string) *analyticsv1.UploadBatch {
	return &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", Version: "test"},
		Events: []*eventv1.CanonicalEvent{{
			Id:      id,
			AgentId: "agent-a",
			HostId:  "host-a",
			Kind:    eventv1.EventKind_EVENT_KIND_EXEC,
		}},
	}
}

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
