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
	Search(context.Context, string, int) ([]json.RawMessage, error)
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

func (i *HTTPIndexer) Search(ctx context.Context, index string, size int) ([]json.RawMessage, error) {
	if i == nil || i.client == nil || i.base == "" {
		return nil, ErrDisabled
	}
	index = strings.TrimSpace(index)
	if index == "" {
		return nil, fmt.Errorf("opensearch index is required")
	}
	if size <= 0 {
		size = 1000
	}
	body, err := json.Marshal(map[string]any{
		"size": size,
		"query": map[string]any{
			"match_all": map[string]any{},
		},
	})
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

func (i *HTTPIndexer) setAuth(req *http.Request) {
	if i.username != "" || i.password != "" {
		req.SetBasicAuth(i.username, i.password)
	}
}
