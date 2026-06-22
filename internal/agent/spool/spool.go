package spool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type Queue struct {
	dir      string
	maxBytes int64
	mu       sync.Mutex

	backpressureCount uint64
	droppedBatches    uint64
	droppedBytes      uint64
	lastError         string
	subs              map[chan struct{}]struct{}
}

type Entry struct {
	ID   string
	Path string
	Size int64
}

type Stats struct {
	QueuedBatches     int
	QueuedBytes       int64
	MaxBytes          int64
	OldestBatchID     string
	NewestBatchID     string
	LastAckedBatchID  string
	WatchSubscribers  uint64
	CorruptBatches    uint64
	BackpressureCount uint64
	DroppedBatches    uint64
	DroppedBytes      uint64
	LastError         string
}

type cursorFile struct {
	LastAcked string `json:"last_acked"`
}

func Open(dir string) (*Queue, error) {
	return OpenWithLimit(dir, 0)
}

func OpenWithLimit(dir string, maxBytes int64) (*Queue, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("spool path is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	q := &Queue{dir: dir, maxBytes: maxBytes, subs: make(map[chan struct{}]struct{})}
	if err := q.cleanupTempFiles(); err != nil {
		return nil, err
	}
	return q, nil
}

func (q *Queue) AppendDataBatch(batch *dataplanev1.DataBatch) (string, error) {
	if batch == nil {
		return "", fmt.Errorf("batch is nil")
	}
	q.mu.Lock()
	id, err := q.nextIDLocked()
	if err != nil {
		q.mu.Unlock()
		return "", err
	}
	ensureHeader(batch).BatchId = id
	fillHeaderCounts(batch)
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(batch)
	if err != nil {
		q.mu.Unlock()
		return "", err
	}
	if err := q.ensureCapacityLocked(int64(len(data) + 1)); err != nil {
		q.mu.Unlock()
		return "", err
	}
	tmp := filepath.Join(q.dir, id+".batch.json.tmp")
	final := q.batchPath(id)
	if err := writeFileSync(tmp, append(data, '\n'), 0o644); err != nil {
		q.mu.Unlock()
		return "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		q.mu.Unlock()
		return "", err
	}
	if err := syncDir(q.dir); err != nil {
		q.mu.Unlock()
		return "", err
	}
	q.notifyLocked()
	q.mu.Unlock()
	return id, nil
}

func (q *Queue) List() ([]Entry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.listLocked()
}

func (q *Queue) LoadDataBatch(id string) (*dataplanev1.DataBatch, error) {
	data, err := os.ReadFile(q.batchPath(id))
	if err != nil {
		return nil, err
	}
	batch := &dataplanev1.DataBatch{}
	if err := protojson.Unmarshal(data, batch); err != nil {
		_ = q.quarantineCorrupt(id, err)
		return nil, err
	}
	return batch, nil
}

func (q *Queue) SnapshotAfter(afterID string) ([]Entry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries, err := q.listLocked()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(afterID) == "" {
		return entries, nil
	}
	out := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.ID > afterID {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (q *Queue) Watch(ctx context.Context, afterID string) (<-chan Entry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	wakeup := make(chan struct{}, 1)
	out := make(chan Entry, 64)
	q.mu.Lock()
	if q.subs == nil {
		q.subs = make(map[chan struct{}]struct{})
	}
	q.subs[wakeup] = struct{}{}
	q.mu.Unlock()

	go func() {
		defer close(out)
		defer func() {
			q.mu.Lock()
			delete(q.subs, wakeup)
			close(wakeup)
			q.mu.Unlock()
		}()
		cursor := strings.TrimSpace(afterID)
		for {
			entries, err := q.SnapshotAfter(cursor)
			if err == nil {
				for _, entry := range entries {
					select {
					case <-ctx.Done():
						return
					case out <- entry:
						cursor = entry.ID
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case _, ok := <-wakeup:
				if !ok {
					return
				}
			}
		}
	}()
	return out, nil
}

func (q *Queue) Ack(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.writeCursorLocked(id); err != nil {
		return err
	}
	if err := os.Remove(q.batchPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDir(q.dir)
}

func (q *Queue) AckThrough(cursor string) error {
	if strings.TrimSpace(cursor) == "" {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	entries, err := q.listLocked()
	if err != nil {
		return err
	}
	if err := q.writeCursorLocked(cursor); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.ID > cursor {
			continue
		}
		if err := os.Remove(q.batchPath(entry.ID)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return syncDir(q.dir)
}

func (q *Queue) writeCursorLocked(id string) error {
	cursor := cursorFile{LastAcked: id}
	data, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(q.dir, "cursor.json.tmp")
	final := filepath.Join(q.dir, "cursor.json")
	if err := writeFileSync(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return syncDir(q.dir)
}

func (q *Queue) Stats() (Stats, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entries, err := q.listLocked()
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{
		MaxBytes:          q.maxBytes,
		WatchSubscribers:  uint64(len(q.subs)),
		CorruptBatches:    q.corruptBatchesLocked(),
		BackpressureCount: q.backpressureCount,
		DroppedBatches:    q.droppedBatches,
		DroppedBytes:      q.droppedBytes,
		LastError:         q.lastError,
	}
	stats.QueuedBatches = len(entries)
	if len(entries) > 0 {
		stats.OldestBatchID = entries[0].ID
		stats.NewestBatchID = entries[len(entries)-1].ID
	}
	for _, entry := range entries {
		stats.QueuedBytes += entry.Size
	}
	stats.LastAckedBatchID = q.lastAckedLocked()
	return stats, nil
}

var ErrBackpressure = errors.New("spool max_bytes exceeded")

func IsBackpressure(err error) bool {
	return errors.Is(err, ErrBackpressure)
}

func (q *Queue) ensureCapacityLocked(addBytes int64) error {
	if q.maxBytes <= 0 {
		return nil
	}
	entries, err := q.listLocked()
	if err != nil {
		return err
	}
	var queued int64
	for _, entry := range entries {
		queued += entry.Size
	}
	if queued+addBytes <= q.maxBytes {
		return nil
	}
	q.backpressureCount++
	q.droppedBatches++
	if addBytes > 0 {
		q.droppedBytes += uint64(addBytes)
	}
	q.lastError = fmt.Sprintf("%v: queued=%d add=%d max=%d", ErrBackpressure, queued, addBytes, q.maxBytes)
	return fmt.Errorf("%w: queued=%d add=%d max=%d", ErrBackpressure, queued, addBytes, q.maxBytes)
}

func (q *Queue) nextIDLocked() (string, error) {
	entries, err := q.listLocked()
	if err != nil {
		return "", err
	}
	next := uint64(1)
	if lastAcked := q.lastAckedLocked(); lastAcked != "" {
		var last uint64
		if _, err := fmt.Sscanf(lastAcked, "%d", &last); err == nil {
			next = last + 1
		}
	}
	if len(entries) > 0 {
		var last uint64
		if _, err := fmt.Sscanf(entries[len(entries)-1].ID, "%d", &last); err == nil && last >= next {
			next = last + 1
		}
	}
	return fmt.Sprintf("%020d", next), nil
}

func (q *Queue) listLocked() ([]Entry, error) {
	matches, err := filepath.Glob(filepath.Join(q.dir, "*.batch.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	entries := make([]Entry, 0, len(matches))
	lastAcked := q.lastAckedLocked()
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(path)
		id := strings.TrimSuffix(name, ".batch.json")
		if lastAcked != "" && id <= lastAcked {
			continue
		}
		entries = append(entries, Entry{ID: id, Path: path, Size: info.Size()})
	}
	return entries, nil
}

func (q *Queue) batchPath(id string) string {
	return filepath.Join(q.dir, id+".batch.json")
}

func (q *Queue) lastAckedLocked() string {
	data, err := os.ReadFile(filepath.Join(q.dir, "cursor.json"))
	if err != nil {
		return ""
	}
	var cursor cursorFile
	if err := json.Unmarshal(data, &cursor); err != nil {
		return ""
	}
	return cursor.LastAcked
}

func (q *Queue) cleanupTempFiles() error {
	matches, err := filepath.Glob(filepath.Join(q.dir, "*.tmp"))
	if err != nil {
		return err
	}
	for _, path := range matches {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (q *Queue) quarantineCorrupt(id string, cause error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	src := q.batchPath(id)
	dst := filepath.Join(q.dir, id+".batch.json.corrupt")
	if err := os.Rename(src, dst); err != nil {
		return err
	}
	q.droppedBatches++
	q.lastError = fmt.Sprintf("quarantined corrupt batch %s: %v", id, cause)
	return syncDir(q.dir)
}

func (q *Queue) corruptBatchesLocked() uint64 {
	matches, err := filepath.Glob(filepath.Join(q.dir, "*.batch.json.corrupt"))
	if err != nil {
		return 0
	}
	return uint64(len(matches))
}

func writeFileSync(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func (q *Queue) notifyLocked() {
	for ch := range q.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func ensureHeader(batch *dataplanev1.DataBatch) *dataplanev1.BatchHeader {
	if batch.Header == nil {
		batch.Header = &dataplanev1.BatchHeader{}
	}
	return batch.Header
}

func fillHeaderCounts(batch *dataplanev1.DataBatch) {
	header := ensureHeader(batch)
	header.EventCount = uint32(len(batch.GetEvents()))
	header.SignalCount = uint32(len(batch.GetSignals()))
	if header.CreatedAtUnixNano == 0 {
		header.CreatedAtUnixNano = time.Now().UTC().UnixNano()
	}
	if len(batch.GetEvents()) > 0 {
		header.EventSeqStart = batch.GetEvents()[0].GetSequence()
		header.EventSeqEnd = batch.GetEvents()[len(batch.GetEvents())-1].GetSequence()
	}
	if len(batch.GetSignals()) > 0 {
		header.SignalSeqStart = batch.GetSignals()[0].GetSequence()
		header.SignalSeqEnd = batch.GetSignals()[len(batch.GetSignals())-1].GetSequence()
	}
}
