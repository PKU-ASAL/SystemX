package uploadworker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/uploader"
)

type Worker struct {
	Queue    *spool.Queue
	Uploader uploader.BatchUploader
	Backoff  Backoff

	mu        sync.Mutex
	lastError string
}

type Backoff struct {
	Initial time.Duration
	Max     time.Duration
}

type Stats struct {
	UploadedBatches  int
	RemainingBatches int
	RemainingBytes   int64
	LastError        string
}

func (w *Worker) DrainOnce(ctx context.Context) (Stats, error) {
	if w.Queue == nil {
		return Stats{}, fmt.Errorf("spool queue is nil")
	}
	if w.Uploader == nil {
		return Stats{}, fmt.Errorf("uploader is nil")
	}
	entries, err := w.Queue.List()
	if err != nil {
		return Stats{}, err
	}
	var stats Stats
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			stats.LastError = ctx.Err().Error()
			return w.withRemaining(stats)
		default:
		}
		batch, err := w.Queue.Load(entry.ID)
		if err != nil {
			stats.LastError = err.Error()
			return w.withRemaining(stats)
		}
		ack, err := w.Uploader.Upload(batch)
		if err != nil {
			stats.LastError = err.Error()
			w.setLastError(stats.LastError)
			return w.withRemaining(stats)
		}
		if ack != nil && ack.GetBatchId() != "" && ack.GetBatchId() != entry.ID {
			stats.LastError = fmt.Sprintf("upload ack batch_id mismatch: got %q want %q", ack.GetBatchId(), entry.ID)
			w.setLastError(stats.LastError)
			return w.withRemaining(stats)
		}
		if err := w.Queue.Ack(entry.ID); err != nil {
			stats.LastError = err.Error()
			return w.withRemaining(stats)
		}
		stats.UploadedBatches++
	}
	stats.LastError = ""
	w.setLastError("")
	return w.withRemaining(stats)
}

func (w *Worker) DrainWithRetry(ctx context.Context) (Stats, error) {
	backoff := w.Backoff.Initial
	if backoff <= 0 {
		backoff = time.Second
	}
	maxBackoff := w.Backoff.Max
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}
	var last Stats
	for {
		stats, err := w.DrainOnce(ctx)
		if err != nil {
			return stats, err
		}
		last = stats
		if stats.RemainingBatches == 0 || stats.LastError == "" {
			return stats, nil
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, ctx.Err()
		case <-timer.C:
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (w *Worker) Stats() (Stats, error) {
	return w.withRemaining(Stats{})
}

func (w *Worker) withRemaining(stats Stats) (Stats, error) {
	queued, err := w.Queue.Stats()
	if err != nil {
		return stats, err
	}
	stats.RemainingBatches = queued.QueuedBatches
	stats.RemainingBytes = queued.QueuedBytes
	if stats.LastError == "" {
		stats.LastError = w.getLastError()
	}
	return stats, nil
}

func (w *Worker) setLastError(err string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastError = err
}

func (w *Worker) getLastError() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastError
}
