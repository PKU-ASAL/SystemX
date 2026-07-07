package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrDisabled = errors.New("opensearch indexing is disabled")

type Document struct {
	Index string
	ID    string
	Body  []byte
}

type Indexer interface {
	Index(context.Context, Document) error
}

type Searcher interface {
	Search(context.Context, SearchRequest) ([]json.RawMessage, error)
}

type SearchRequest struct {
	Index     string
	Size      int
	Offset    int
	Labels    map[string]string
	Exact     map[string]string
	Bool      map[string]bool
	SortField string
	SortDesc  bool
}

type DisabledIndexer struct{}

func (DisabledIndexer) Index(context.Context, Document) error {
	return ErrDisabled
}

type NoopIndexer struct{}

func (NoopIndexer) Index(context.Context, Document) error {
	return nil
}

type HTTPIndexer struct {
	base     string
	username string
	password string
	client   *http.Client
}

func NewHTTPIndexer(baseURL string) (*HTTPIndexer, error) {
	return NewHTTPIndexerWithAuth(baseURL, "", "")
}

func NewHTTPIndexerWithAuth(baseURL, username, password string) (*HTTPIndexer, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, ErrDisabled
	}
	return &HTTPIndexer{
		base:     baseURL,
		username: strings.TrimSpace(username),
		password: password,
		client:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (i *HTTPIndexer) Index(ctx context.Context, doc Document) error {
	if i == nil || i.client == nil || i.base == "" {
		return ErrDisabled
	}
	if doc.Index == "" || doc.ID == "" {
		return fmt.Errorf("opensearch index and id are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, i.base+"/"+doc.Index+"/_doc/"+doc.ID, bytes.NewReader(doc.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	i.setAuth(req)
	resp, err := i.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("opensearch index status %s", resp.Status)
	}
	return nil
}

func (i *HTTPIndexer) Search(ctx context.Context, search SearchRequest) ([]json.RawMessage, error) {
	if i == nil || i.client == nil || i.base == "" {
		return nil, ErrDisabled
	}
	index := strings.TrimSpace(search.Index)
	if index == "" {
		return nil, fmt.Errorf("opensearch index is required")
	}
	size := search.Size
	if size <= 0 {
		size = 100
	}
	bodyMap := map[string]any{
		"size":  size,
		"from":  max(search.Offset, 0),
		"query": searchQuery(search),
	}
	if sortField := strings.TrimSpace(search.SortField); sortField != "" {
		order := "asc"
		if search.SortDesc {
			order = "desc"
		}
		bodyMap["sort"] = []map[string]any{{sortField: map[string]any{"order": order, "unmapped_type": "keyword"}}}
	}
	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i.base+"/"+index+"/_search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	i.setAuth(req)
	resp, err := i.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("opensearch search status %s", resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result struct {
		Hits struct {
			Hits []struct {
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	out := make([]json.RawMessage, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		if len(hit.Source) > 0 {
			out = append(out, hit.Source)
		}
	}
	return out, nil
}

func searchQuery(search SearchRequest) map[string]any {
	filters := make([]map[string]any, 0, len(search.Labels)+len(search.Exact)+len(search.Bool))
	for key, value := range search.Labels {
		if key = strings.TrimSpace(key); key != "" {
			filters = append(filters, termFilter("labels."+key+".keyword", value))
		}
	}
	for field, value := range search.Exact {
		if field = strings.TrimSpace(field); field != "" {
			filters = append(filters, termFilter(field+".keyword", value))
		}
	}
	for field, value := range search.Bool {
		if field = strings.TrimSpace(field); field != "" {
			filters = append(filters, map[string]any{"term": map[string]any{field: value}})
		}
	}
	if len(filters) == 0 {
		return map[string]any{"match_all": map[string]any{}}
	}
	return map[string]any{"bool": map[string]any{"filter": filters}}
}

func termFilter(field, value string) map[string]any {
	return map[string]any{"term": map[string]any{field: value}}
}

func (i *HTTPIndexer) setAuth(req *http.Request) {
	if i.username != "" || i.password != "" {
		req.SetBasicAuth(i.username, i.password)
	}
}
