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

func TestDrainOnceKeepsBatchOnAckIDMismatch(t *testing.T) {
	queue := openQueue(t)
	mustAppend(t, queue, "event-1")
	up := &recordingUploader{ackBatchID: "different-batch"}
	worker := &Worker{Queue: queue, Uploader: up}
	stats, err := worker.DrainOnce(context.Background())
	if err != nil {
		t.Fatalf("DrainOnce() error = %v", err)
	}
	if stats.UploadedBatches != 0 || stats.RemainingBatches != 1 || stats.LastError == "" {
		t.Fatalf("stats = %+v", stats)
	}
	entries, err := queue.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
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

func TestDrainWithRetryCapsAtMaxBackoff(t *testing.T) {
	queue := openQueue(t)
	mustAppend(t, queue, "event-1")
	up := &recordingUploader{failBeforeSuccess: 4}
	worker := &Worker{
		Queue:    queue,
		Uploader: up,
		Backoff:  Backoff{Initial: 5 * time.Millisecond, Max: 10 * time.Millisecond},
	}

	start := time.Now()
	stats, err := worker.DrainWithRetry(context.Background())
	if err != nil {
		t.Fatalf("DrainWithRetry() error = %v", err)
	}
	if stats.UploadedBatches != 1 || stats.RemainingBatches != 0 || stats.LastError != "" {
		t.Fatalf("stats = %+v", stats)
	}
	if up.attempts != 5 {
		t.Fatalf("attempts = %d, want 5", up.attempts)
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("elapsed = %s, want at least capped backoff window", elapsed)
	}
	if elapsed := time.Since(start); elapsed > 120*time.Millisecond {
		t.Fatalf("elapsed = %s, unexpectedly high for capped backoff", elapsed)
	}
}

func TestDrainWithRetryStopsOnContextCancel(t *testing.T) {
	queue := openQueue(t)
	mustAppend(t, queue, "event-1")
	up := &recordingUploader{failBeforeSuccess: 100}
	worker := &Worker{
		Queue:    queue,
		Uploader: up,
		Backoff:  Backoff{Initial: 50 * time.Millisecond, Max: 50 * time.Millisecond},
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)

	start := time.Now()
	stats, err := worker.DrainWithRetry(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DrainWithRetry() error = %v, want context.Canceled", err)
	}
	if stats.RemainingBatches != 1 || stats.LastError == "" {
		t.Fatalf("stats = %+v", stats)
	}
	if up.attempts != 1 {
		t.Fatalf("attempts = %d, want 1 before cancellation", up.attempts)
	}
	if elapsed := time.Since(start); elapsed > 80*time.Millisecond {
		t.Fatalf("elapsed = %s, cancel should stop retry promptly", elapsed)
	}
}

func TestResumeOnceAcksThroughCursor(t *testing.T) {
	queue := openQueue(t)
	id1 := mustAppend(t, queue, "event-1")
	id2 := mustAppend(t, queue, "event-2")
	mustAppend(t, queue, "event-3")
	worker := &Worker{
		Queue:        queue,
		ResumeSource: staticResumeSource{cursor: id2},
	}

	stats, err := worker.ResumeOnce(context.Background())
	if err != nil {
		t.Fatalf("ResumeOnce() error = %v", err)
	}
	if stats.RemainingBatches != 1 || stats.LastError != "" {
		t.Fatalf("stats = %+v", stats)
	}
	entries, err := queue.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].ID <= id2 || entries[0].ID == id1 {
		t.Fatalf("remaining entry = %+v, cursor = %s", entries[0], id2)
	}
}

func TestResumeOnceNoCursorIsNoop(t *testing.T) {
	queue := openQueue(t)
	mustAppend(t, queue, "event-1")
	worker := &Worker{
		Queue:        queue,
		ResumeSource: staticResumeSource{},
	}

	stats, err := worker.ResumeOnce(context.Background())
	if err != nil {
		t.Fatalf("ResumeOnce() error = %v", err)
	}
	if stats.RemainingBatches != 1 || stats.LastError != "" {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestResumeOnceKeepsBatchesOnResumeError(t *testing.T) {
	queue := openQueue(t)
	mustAppend(t, queue, "event-1")
	resumeErr := errors.New("resume unavailable")
	worker := &Worker{
		Queue:        queue,
		ResumeSource: staticResumeSource{err: resumeErr},
	}

	stats, err := worker.ResumeOnce(context.Background())
	if err != nil {
		t.Fatalf("ResumeOnce() error = %v", err)
	}
	if stats.RemainingBatches != 1 || stats.LastError != resumeErr.Error() {
		t.Fatalf("stats = %+v", stats)
	}
	entries, err := queue.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
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

func mustAppend(t *testing.T, q *spool.Queue, eventID string) string {
	t.Helper()
	id, err := q.Append(&analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "agent-a", HostId: "host-a", TenantId: "default", Version: "test"},
		Events: []*eventv1.CanonicalEvent{{
			Id:       eventID,
			AgentId:  "agent-a",
			HostId:   "host-a",
			Behavior: "process.exec",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type recordingUploader struct {
	ids               []string
	failAfter         int
	failBeforeSuccess int
	attempts          int
	ackBatchID        string
}

func (u *recordingUploader) Upload(batch *analyticsv1.UploadBatch) (*analyticsv1.UploadAck, error) {
	u.attempts++
	if u.failBeforeSuccess > 0 && u.attempts <= u.failBeforeSuccess {
		return nil, errors.New("temporary upload failure")
	}
	if u.failAfter > 0 && len(u.ids) >= u.failAfter {
		return nil, errors.New("upload failed")
	}
	u.ids = append(u.ids, batch.GetEvents()[0].GetId())
	ackID := u.ackBatchID
	if ackID == "" {
		ackID = batch.GetBatchId()
	}
	return &analyticsv1.UploadAck{Ok: true, BatchId: ackID}, nil
}

type staticResumeSource struct {
	cursor string
	err    error
}

func (s staticResumeSource) ResumeCursor(context.Context) (string, error) {
	return s.cursor, s.err
}
