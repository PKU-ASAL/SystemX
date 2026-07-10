package managerapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platformopensearch "github.com/sysarmor/sysarmor-next-project/internal/platform/opensearch"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
)

func TestSearchFieldsReturnsAllowlistedTelemetryFields(t *testing.T) {
	handler := NewServer(&store.Store{}).Handler()

	rec := get(t, handler, "/api/v1/search/fields?index=events-*,signals-*")

	for _, want := range []string{
		`"indexes":["sysarmor-events","sysarmor-signals"]`,
		`"name":"@timestamp"`,
		`"name":"host.name"`,
		`"name":"event.severity"`,
		`"searchable":true`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("fields response missing %s: %s", want, rec.Body.String())
		}
	}
}

func TestSearchTelemetryReturnsDiscoverRows(t *testing.T) {
	searcher := &recordingSearcher{docs: map[string][]json.RawMessage{
		"sysarmor-events": {
			json.RawMessage(`{"id":"evt-a","@timestamp":"2026-07-08T21:04:18Z","host":{"name":"prod-api-01"},"event":{"kind":"event","summary":"process execution","tactic":"Execution","severity":"medium"}}`),
			json.RawMessage(`{"id":"evt-b","@timestamp":"2026-07-08T21:05:18Z","host":{"name":"prod-db-01"},"event":{"kind":"event","summary":"file write","severity":"info"}}`),
		},
		"sysarmor-signals": {
			json.RawMessage(`{"id":"sig-a","@timestamp":"2026-07-08T21:06:18Z","host":{"name":"prod-api-01"},"event":{"kind":"signal","summary":"credential access","tactic":"CredentialAccess","severity":"critical"}}`),
		},
	}}
	handler := NewServerWithSearch(&store.Store{}, "", searcher).Handler()
	body := `{"indexes":["sysarmor-events","sysarmor-signals"],"query":"host.name:prod-api-01","time":{"field":"@timestamp","from":"2026-07-08T21:00:00Z","to":"2026-07-08T21:10:00Z"},"limit":50,"offset":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("search status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{
		`"total":2`,
		`"index":"sysarmor-events"`,
		`"id":"evt-a"`,
		`"index":"sysarmor-signals"`,
		`"id":"sig-a"`,
		`"summary":"credential access"`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("search response missing %s: %s", want, rec.Body.String())
		}
	}
	if strings.Contains(rec.Body.String(), `"id":"evt-b"`) {
		t.Fatalf("search response included filtered row: %s", rec.Body.String())
	}
	if len(searcher.requests) != 2 {
		t.Fatalf("search requests = %d, want 2", len(searcher.requests))
	}
	if searcher.requests[0].Index != "sysarmor-events" || searcher.requests[1].Index != "sysarmor-signals" {
		t.Fatalf("search indexes = %#v", searcher.requests)
	}
}

func TestSearchTelemetryRejectsUnsupportedField(t *testing.T) {
	handler := NewServerWithSearch(&store.Store{}, "", &recordingSearcher{}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(`{"indexes":["sysarmor-events"],"query":"process.args:curl"}`))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("search status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported search field") {
		t.Fatalf("search error mismatch: %s", rec.Body.String())
	}
}

func TestSearchHistogramReturnsTimeBuckets(t *testing.T) {
	searcher := &recordingSearcher{docs: map[string][]json.RawMessage{
		"sysarmor-signals": {
			json.RawMessage(`{"id":"sig-a","@timestamp":"2026-07-08T21:01:00Z","host":{"name":"prod-api-01"},"event":{"kind":"signal","severity":"critical","summary":"credential access"}}`),
			json.RawMessage(`{"id":"sig-b","@timestamp":"2026-07-08T21:06:00Z","host":{"name":"prod-api-01"},"event":{"kind":"signal","severity":"high","summary":"lateral movement"}}`),
		},
	}}
	handler := NewServerWithSearch(&store.Store{}, "", searcher).Handler()
	body := `{"indexes":["sysarmor-signals"],"time":{"field":"@timestamp","from":"2026-07-08T21:00:00Z","to":"2026-07-08T21:10:00Z"},"bucket_count":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search/histogram", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("histogram status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{
		`"total":1`,
		`"signals":1`,
		`"events":0`,
		`"start":"2026-07-08T21:00:00Z"`,
		`"start":"2026-07-08T21:05:00Z"`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("histogram response missing %s: %s", want, rec.Body.String())
		}
	}
}

type recordingSearcher struct {
	docs     map[string][]json.RawMessage
	requests []platformopensearch.SearchRequest
}

func (s *recordingSearcher) Search(_ context.Context, search platformopensearch.SearchRequest) ([]json.RawMessage, error) {
	s.requests = append(s.requests, search)
	if s.docs == nil {
		return nil, nil
	}
	return append([]json.RawMessage(nil), s.docs[search.Index]...), nil
}
