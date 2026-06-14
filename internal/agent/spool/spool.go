package spool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type Queue struct {
	dir string
	mu  sync.Mutex
}

type Entry struct {
	ID   string
	Path string
	Size int64
}

type Stats struct {
	QueuedBatches int
	QueuedBytes   int64
}

type cursorFile struct {
	LastAcked string `json:"last_acked"`
}

func Open(dir string) (*Queue, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("spool path is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Queue{dir: dir}, nil
}

func (q *Queue) Append(batch *analyticsv1.UploadBatch) (string, error) {
	if batch == nil {
		return "", fmt.Errorf("batch is nil")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	id, err := q.nextIDLocked()
	if err != nil {
		return "", err
	}
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(batch)
	if err != nil {
		return "", err
	}
	tmp := filepath.Join(q.dir, id+".batch.json.tmp")
	final := q.batchPath(id)
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return id, nil
}

func (q *Queue) List() ([]Entry, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.listLocked()
}

func (q *Queue) Load(id string) (*analyticsv1.UploadBatch, error) {
	data, err := os.ReadFile(q.batchPath(id))
	if err != nil {
		return nil, err
	}
	batch := &analyticsv1.UploadBatch{}
	if err := protojson.Unmarshal(data, batch); err != nil {
		return nil, err
	}
	return batch, nil
}

func (q *Queue) Ack(id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := os.Remove(q.batchPath(id)); err != nil && !os.IsNotExist(err) {
		return err
	}
	cursor := cursorFile{LastAcked: id}
	data, err := json.MarshalIndent(cursor, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(q.dir, "cursor.json"), append(data, '\n'), 0o644)
}

func (q *Queue) Stats() (Stats, error) {
	entries, err := q.List()
	if err != nil {
		return Stats{}, err
	}
	var stats Stats
	stats.QueuedBatches = len(entries)
	for _, entry := range entries {
		stats.QueuedBytes += entry.Size
	}
	return stats, nil
}

func (q *Queue) nextIDLocked() (string, error) {
	entries, err := q.listLocked()
	if err != nil {
		return "", err
	}
	next := uint64(1)
	if len(entries) > 0 {
		var last uint64
		if _, err := fmt.Sscanf(entries[len(entries)-1].ID, "%d", &last); err == nil {
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
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		name := filepath.Base(path)
		id := strings.TrimSuffix(name, ".batch.json")
		entries = append(entries, Entry{ID: id, Path: path, Size: info.Size()})
	}
	return entries, nil
}

func (q *Queue) batchPath(id string) string {
	return filepath.Join(q.dir, id+".batch.json")
}
