package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/dataappend"
)

type spoolUploader struct {
	store        *localstore.Store
	sender       dataappend.BatchSender
	fromSequence uint64
}

func (u *spoolUploader) Run(ctx context.Context) {
	backoff := time.Second
	for {
		err := u.uploadAvailable(ctx)
		if ctx.Err() != nil {
			return
		}
		delay := 500 * time.Millisecond
		if err != nil {
			delay = backoff
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		} else {
			backoff = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (u *spoolUploader) uploadAvailable(ctx context.Context) error {
	if u == nil || u.store == nil || u.sender == nil {
		return fmt.Errorf("spool uploader is not configured")
	}
	checkpoint, err := u.store.Checkpoint(ctx)
	if err != nil {
		return err
	}
	batches, err := u.store.ReadBatches(ctx, localstore.ReadOptions{Limit: 1000, FromSequence: u.fromSequence})
	if err != nil {
		return err
	}
	for _, stored := range batches {
		if beforeCheckpoint(stored.Position, checkpoint) {
			continue
		}
		ack, err := u.sender.SendBatch(stored.Batch)
		if err != nil {
			return err
		}
		if !dataappend.AckCommitted(ack) {
			return fmt.Errorf("batch %s not committed: %s", stored.Position.BatchID, ack.GetMessage())
		}
		if err := u.store.SaveCheckpoint(ctx, localstore.Checkpoint{SegmentID: stored.Position.SegmentID, RecordOffset: stored.Position.RecordOffset, LastBatchID: stored.Position.BatchID}); err != nil {
			return err
		}
	}
	return nil
}

func beforeCheckpoint(position localstore.Position, checkpoint localstore.Checkpoint) bool {
	if position.SegmentID < checkpoint.SegmentID {
		return true
	}
	return position.SegmentID == checkpoint.SegmentID && position.RecordOffset < checkpoint.RecordOffset
}
