package context

import "sync"

type Process struct {
	StableID     string
	SensorExecID string
	PID          uint32
	PPID         uint32
	Binary       string
	LineageID    string
}

type Table struct {
	mu       sync.RWMutex
	byPID    map[uint32]Process
	byStable map[string]Process
	bySensor map[string]Process
}

func NewTable() *Table {
	return &Table{
		byPID:    make(map[uint32]Process),
		byStable: make(map[string]Process),
		bySensor: make(map[string]Process),
	}
}

func (t *Table) Upsert(proc Process) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.byPID[proc.PID] = proc
	t.byStable[proc.StableID] = proc
	if proc.SensorExecID != "" {
		t.bySensor[proc.SensorExecID] = proc
	}
}

func (t *Table) ByPID(pid uint32) (Process, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	proc, ok := t.byPID[pid]
	return proc, ok
}

func (t *Table) ByStableID(stableID string) (Process, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	proc, ok := t.byStable[stableID]
	return proc, ok
}

func (t *Table) BySensorExecID(execID string) (Process, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	proc, ok := t.bySensor[execID]
	return proc, ok
}
