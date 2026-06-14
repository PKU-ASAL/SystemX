package uploader

import (
	"context"
	"strings"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/ringbuffer"
)

type recordingUploader struct {
	batches []*analyticsv1.UploadBatch
}

func (u *recordingUploader) Upload(batch *analyticsv1.UploadBatch) error {
	u.batches = append(u.batches, batch)
	return nil
}

func TestStreamJSONLBatchesAndAssignsRawRefs(t *testing.T) {
	input := strings.Join([]string{
		`{"mono_ns":"1","kind":"EVENT_KIND_EXEC","proc":{"pid":1,"binary":"/usr/bin/java-web","start_time_ns":"11"}}`,
		`{"mono_ns":"2","kind":"EVENT_KIND_EXEC","proc":{"pid":2,"ppid":1,"binary":"/bin/bash","argv":["/bin/bash"],"start_time_ns":"22"}}`,
		`{"mono_ns":"3","kind":"EVENT_KIND_WRITE","proc":{"pid":2,"ppid":1,"binary":"/bin/bash","start_time_ns":"22"},"object":{"path":"/dev/shm/x.sh"}}`,
	}, "\n") + "\n"
	rec := &recordingUploader{}
	ring := ringbuffer.New(8)

	stats, err := StreamJSONL(context.Background(), strings.NewReader(input), rec, StreamOptions{
		AgentID:       "agent-a",
		HostID:        "host-a",
		Scenario:      "scenario-a",
		Version:       "test",
		BatchSize:     2,
		FlushInterval: time.Hour,
		RawRing:       ring,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Events != 3 || stats.Batches != 2 {
		t.Fatalf("stats = %#v, want 3 events and 2 batches", stats)
	}
	if len(rec.batches) != 2 {
		t.Fatalf("uploaded batches = %d, want 2", len(rec.batches))
	}
	if rec.batches[0].GetAgent().GetAgentId() != "agent-a" {
		t.Fatalf("agent metadata missing: %#v", rec.batches[0].GetAgent())
	}
	firstRef := rec.batches[0].GetEvents()[0].GetRawRef()
	if firstRef == "" {
		t.Fatal("first event raw ref is empty")
	}
	if _, ok := ring.Get(firstRef); !ok {
		t.Fatalf("raw ref %q not found in ring", firstRef)
	}
}

func TestStreamJSONLStoresTetragonRawLineBehindRef(t *testing.T) {
	input := `{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-o /dev/shm/x.sh","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}` + "\n"
	rec := &recordingUploader{}
	ring := ringbuffer.New(8)

	stats, err := StreamJSONL(context.Background(), strings.NewReader(input), rec, StreamOptions{
		AgentID:       "agent-a",
		HostID:        "host-a",
		Scenario:      "scenario-a",
		Version:       "test",
		BatchSize:     10,
		FlushInterval: time.Hour,
		RawRing:       ring,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Events != 2 {
		t.Fatalf("events = %d, want exec + inferred write", stats.Events)
	}
	ref := rec.batches[0].GetEvents()[0].GetRawRef()
	if ref == "" || strings.HasPrefix(ref, "{") {
		t.Fatalf("raw ref = %q, want compact ring ref", ref)
	}
	entry, ok := ring.Get(ref)
	if !ok {
		t.Fatalf("raw ref %q missing from ring", ref)
	}
	if !strings.Contains(string(entry.Data), `"process_exec"`) {
		t.Fatalf("raw entry = %s", string(entry.Data))
	}
}
