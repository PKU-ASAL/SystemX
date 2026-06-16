package opensearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPIndexerIndexesDocument(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	indexer, err := NewHTTPIndexer(server.URL)
	if err != nil {
		t.Fatalf("NewHTTPIndexer() error = %v", err)
	}
	if err := indexer.Index(context.Background(), Document{Index: "sysarmor-events", ID: "ev-a", Body: []byte(`{"id":"ev-a"}`)}); err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	if gotPath != "/sysarmor-events/_doc/ev-a" {
		t.Fatalf("path = %s", gotPath)
	}
}
