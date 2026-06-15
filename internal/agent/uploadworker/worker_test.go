package uploadworker

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
)

func TestDrainOnceUploadsOldestFirstAndAcks(t *testing.T) {
	queue := openQueue(t)
	mustAppend(t, queue, "event-1")
	mustAppend(t, queue, "event-2")
	up := &recordingUploader{}
	worker := &Worker{Queue: queue, Uploader: up}
	stats, err := worker.DrainOnce(context.Background())
	if err != nil {
		t.Fatalf("DrainOnce() error = %v", err)
	}
	if stats.UploadedBatches != 2 || stats.RemainingBatches != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if got := up.ids; len(got) != 2 || got[0] != "event-1" || got[1] != "event-2" {
		t.Fatalf("uploaded ids = %v", got)
	}
}

func TestDrainOnceStopsOnFailureAndKeepsBatch(t *testing.T) {
	queue := openQueue(t)
	mustAppend(t, queue, "event-1")
	mustAppend(t, queue, "event-2")
	up := &recordingUploader{failAfter: 1}
	worker := &Worker{Queue: queue, Uploader: up}
	stats, err := worker.DrainOnce(context.Background())
	if err != nil {
		t.Fatalf("DrainOnce() error = %v", err)
	}
	if stats.UploadedBatches != 1 || stats.RemainingBatches != 1 || stats.LastError == "" {
		t.Fatalf("stats = %+v", stats)
	}
	entries, err := queue.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
	}
	batch, err := queue.Load(entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if batch.GetEvents()[0].GetId() != "event-2" {
		t.Fatalf("remaining batch = %+v", batch)
	}
}

func TestDrainWithRetryBacksOffAndRecovers(t *testing.T) {
	queue := openQueue(t)
	mustAppend(t, queue, "event-1")
	up := &recordingUploader{failBeforeSuccess: 2}
	worker := &Worker{
		Queue:    queue,
		Uploader: up,
		Backoff:  Backoff{Initial: 10 * time.Millisecond, Max: 20 * time.Millisecond},
	}

	start := time.Now()
	stats, err := worker.DrainWithRetry(context.Background())
	if err != nil {
		t.Fatalf("DrainWithRetry() error = %v", err)
	}
	if stats.UploadedBatches != 1 || stats.RemainingBatches != 0 || stats.LastError != "" {
		t.Fatalf("stats = %+v", stats)
	}
	if up.attempts != 3 {
		t.Fatalf("attempts = %d, want 3", up.attempts)
	}
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Fatalf("elapsed = %s, want at least 25ms of retry backoff", elapsed)
	}
}

func openQueue(t *testing.T) *spool.Queue {
	t.Helper()
	q, err := spool.Open(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func mustAppend(t *testing.T, q *spool.Queue, eventID string) {
	t.Helper()
	if _, err := q.Append(&analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", Version: "test"},
		Events: []*eventv1.CanonicalEvent{{
			Id:      eventID,
			AgentId: "agent-a",
			HostId:  "host-a",
			Kind:    eventv1.EventKind_EVENT_KIND_EXEC,
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

type recordingUploader struct {
	ids               []string
	failAfter         int
	failBeforeSuccess int
	attempts          int
}

func (u *recordingUploader) Upload(batch *analyticsv1.UploadBatch) error {
	u.attempts++
	if u.failBeforeSuccess > 0 && u.attempts <= u.failBeforeSuccess {
		return errors.New("temporary upload failure")
	}
	if u.failAfter > 0 && len(u.ids) >= u.failAfter {
		return errors.New("upload failed")
	}
	u.ids = append(u.ids, batch.GetEvents()[0].GetId())
	return nil
}
