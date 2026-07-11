package ingestworker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	platformopensearch "github.com/sysarmor/sysarmor-next-project/internal/platform/opensearch"
)

type recordingSearcher struct {
	requests []platformopensearch.SearchRequest
	docs     map[string][]json.RawMessage
}

func (s *recordingSearcher) Search(_ context.Context, request platformopensearch.SearchRequest) ([]json.RawMessage, error) {
	s.requests = append(s.requests, request)
	return s.docs[request.Index], nil
}

func TestOpenSearchHistoryReadsTenantScopeAndWindow(t *testing.T) {
	searcher := &recordingSearcher{docs: map[string][]json.RawMessage{
		"sysarmor-events":  {json.RawMessage(`{"id":"ev-history","labels":{"scenario":"a"},"@timestamp":"2026-07-12T00:05:00Z"}`)},
		"sysarmor-signals": {json.RawMessage(`{"id":"sig-history","name":"payload_dropped","where":"SIGNAL_WHERE_ENDPOINT","labels":{"scenario":"a"},"@timestamp":"2026-07-12T00:05:00Z"}`)},
	}}
	upper := time.Date(2026, 7, 12, 0, 10, 0, 0, time.UTC)
	events, signals, err := NewOpenSearchHistory(searcher).Read(context.Background(), "tenant-a", map[string]string{"scenario": "a"}, upper.Add(-15*time.Minute), upper)
	if err != nil || len(events) != 1 || len(signals) != 1 {
		t.Fatalf("Read() events=%d signals=%d error=%v", len(events), len(signals), err)
	}
	if len(searcher.requests) != 2 {
		t.Fatalf("requests = %d", len(searcher.requests))
	}
	for _, request := range searcher.requests {
		if request.Exact["tenant_id"] != "tenant-a" || request.Labels["scenario"] != "a" || request.TimeField != "@timestamp" || request.TimeFrom == "" || request.TimeTo == "" {
			t.Fatalf("unbounded request = %+v", request)
		}
	}
}

func TestMergeAnalysisInputsDeduplicatesCurrentBatch(t *testing.T) {
	events := mergeEvents([]*eventv1.CanonicalEvent{{Id: "ev-a"}}, []*eventv1.CanonicalEvent{{Id: "ev-a"}, {Id: "ev-b"}})
	signals := mergeSignals([]*signalv1.Signal{{Id: "sig-a"}}, []*signalv1.Signal{{Id: "sig-a"}, {Id: "sig-b"}})
	if len(events) != 2 || len(signals) != 2 {
		t.Fatalf("events=%d signals=%d", len(events), len(signals))
	}
}
