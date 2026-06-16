package opensearch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

type DisabledIndexer struct{}

func (DisabledIndexer) Index(context.Context, Document) error {
	return ErrDisabled
}

type NoopIndexer struct{}

func (NoopIndexer) Index(context.Context, Document) error {
	return nil
}

type HTTPIndexer struct {
	base   string
	client *http.Client
}

func NewHTTPIndexer(baseURL string) (*HTTPIndexer, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, ErrDisabled
	}
	return &HTTPIndexer{
		base:   baseURL,
		client: &http.Client{Timeout: 10 * time.Second},
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
