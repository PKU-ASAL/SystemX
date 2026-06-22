package databatchworker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/dataappend"
)

type Worker struct {
	Queue        *spool.Queue
	Uploader     dataappend.BatchAppender
	ResumeSource ResumeSource
	Backoff      Backoff

	mu        sync.Mutex
	lastError string
}

type ResumeSource interface {
	ResumeCursor(ctx context.Context) (string, error)
}

type Backoff struct {
	Initial time.Duration
	Max     time.Duration
}

type Stats struct {
	AppendedBatches  int
	RejectedBatches  int
	RemainingBatches int
	RemainingBytes   int64
	RetryAfter       time.Duration
	LastError        string
}

func (w *Worker) ResumeOnce(ctx context.Context) (Stats, error) {
	if w.Queue == nil {
		return Stats{}, fmt.Errorf("spool queue is nil")
	}
	if w.ResumeSource == nil {
		return w.withRemaining(Stats{})
	}
	cursor, err := w.ResumeSource.ResumeCursor(ctx)
	if err != nil {
		stats, statErr := w.withRemaining(Stats{LastError: err.Error()})
		w.setLastError(err.Error())
		return stats, statErr
	}
	if cursor == "" {
		return w.withRemaining(Stats{})
	}
	if err := w.Queue.AckThrough(cursor); err != nil {
		stats, statErr := w.withRemaining(Stats{LastError: err.Error()})
		w.setLastError(err.Error())
		return stats, statErr
	}
	w.setLastError("")
	return w.withRemaining(Stats{})
}

func (w *Worker) DrainOnce(ctx context.Context) (Stats, error) {
	if w.Queue == nil {
		return Stats{}, fmt.Errorf("spool queue is nil")
	}
	if w.Uploader == nil {
		return Stats{}, fmt.Errorf("batch appender is nil")
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
		batch, err := w.Queue.LoadDataBatch(entry.ID)
		if err != nil {
			stats.LastError = err.Error()
			return w.withRemaining(stats)
		}
		ack, err := w.Uploader.AppendBatch(batch)
		if err != nil {
			if dataappend.AckRetryable(ack) {
				if ack.GetRetryAfterMs() > 0 {
					stats.RetryAfter = time.Duration(ack.GetRetryAfterMs()) * time.Millisecond
				}
				message := err.Error()
				if ack.GetMessage() != "" {
					message = ack.GetMessage()
				}
				stats.LastError = message
				w.setLastError(stats.LastError)
				return w.withRemaining(stats)
			}
			if dataappend.AckTerminalRejected(ack) {
				if ack.GetBatchId() != "" && ack.GetBatchId() != entry.ID {
					stats.LastError = fmt.Sprintf("data ack batch_id mismatch: got %q want %q", ack.GetBatchId(), entry.ID)
					w.setLastError(stats.LastError)
					return w.withRemaining(stats)
				}
				if err := w.Queue.Ack(entry.ID); err != nil {
					stats.LastError = err.Error()
					return w.withRemaining(stats)
				}
				stats.RejectedBatches++
				stats.LastError = ack.GetMessage()
				continue
			}
			stats.LastError = err.Error()
			w.setLastError(stats.LastError)
			return w.withRemaining(stats)
		}
		if ack != nil && ack.GetBatchId() != "" && ack.GetBatchId() != entry.ID {
			stats.LastError = fmt.Sprintf("data ack batch_id mismatch: got %q want %q", ack.GetBatchId(), entry.ID)
			w.setLastError(stats.LastError)
			return w.withRemaining(stats)
		}
		if !dataappend.AckCommitted(ack) {
			if dataappend.AckRetryable(ack) {
				message := "missing data ack"
				if ack != nil {
					message = ack.GetMessage()
					if ack.GetRetryAfterMs() > 0 {
						stats.RetryAfter = time.Duration(ack.GetRetryAfterMs()) * time.Millisecond
					}
				}
				stats.LastError = fmt.Sprintf("data append retryable: %s", message)
				w.setLastError(stats.LastError)
				return w.withRemaining(stats)
			}
			if !dataappend.AckTerminalRejected(ack) {
				message := "missing data ack"
				if ack != nil {
					message = ack.GetMessage()
				}
				stats.LastError = fmt.Sprintf("data append rejected: %s", message)
				w.setLastError(stats.LastError)
				return w.withRemaining(stats)
			}
			stats.RejectedBatches++
		}
		if err := w.Queue.Ack(entry.ID); err != nil {
			stats.LastError = err.Error()
			return w.withRemaining(stats)
		}
		if dataappend.AckCommitted(ack) {
			stats.AppendedBatches++
		}
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
		delay := backoff
		if stats.RetryAfter > 0 {
			delay = stats.RetryAfter
		}
		timer := time.NewTimer(delay)
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
