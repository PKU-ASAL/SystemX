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

func TestHTTPIndexerSearchesDocuments(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		_, _ = w.Write([]byte(`{"hits":{"hits":[{"_source":{"id":"ev-a"}},{"_source":{"id":"ev-b"}}]}}`))
	}))
	defer server.Close()

	indexer, err := NewHTTPIndexer(server.URL)
	if err != nil {
		t.Fatalf("NewHTTPIndexer() error = %v", err)
	}
	docs, err := indexer.Search(context.Background(), "sysarmor-events", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if gotPath != "/sysarmor-events/_search" {
		t.Fatalf("path = %s", gotPath)
	}
	if len(docs) != 2 || string(docs[0]) != `{"id":"ev-a"}` {
		t.Fatalf("docs = %s", docs)
	}
}

func TestHTTPIndexerUsesBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "admin" || password != "admin" {
			t.Fatalf("basic auth = %q/%q ok=%t", username, password, ok)
		}
		_, _ = w.Write([]byte(`{"hits":{"hits":[]}}`))
	}))
	defer server.Close()

	indexer, err := NewHTTPIndexerWithAuth(server.URL, "admin", "admin")
	if err != nil {
		t.Fatalf("NewHTTPIndexerWithAuth() error = %v", err)
	}
	if _, err := indexer.Search(context.Background(), "sysarmor-events", 10); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
}
