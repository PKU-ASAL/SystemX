package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
)

type exportPipeline struct {
	store        *localstore.Store
	exporter     Exporter
	fromSequence uint64
}

func (u *exportPipeline) Run(ctx context.Context) {
	defer u.exporter.Close()
	backoff := time.Second
	for {
		err := u.exportAvailable(ctx)
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

func (u *exportPipeline) exportAvailable(ctx context.Context) error {
	if u == nil || u.store == nil || u.exporter == nil {
		return fmt.Errorf("export pipeline is not configured")
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
		result, err := u.exporter.Export(ctx, stored.Batch)
		if err != nil {
			return err
		}
		if !result.Committed {
			return fmt.Errorf("batch %s was not committed", stored.Position.BatchID)
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
