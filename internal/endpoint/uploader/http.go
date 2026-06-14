package uploader

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type HTTPUploader struct {
	manager string
	client  *http.Client
	token   string
}

func NewHTTPUploader(manager string) *HTTPUploader {
	return NewHTTPUploaderWithTimeout(manager, 10*time.Second)
}

func NewHTTPUploaderWithTimeout(manager string, timeout time.Duration) *HTTPUploader {
	return NewHTTPUploaderWithOptions(manager, timeout, "")
}

func NewHTTPUploaderWithOptions(manager string, timeout time.Duration, token string) *HTTPUploader {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPUploader{
		manager: normalizeManagerURL(manager),
		client:  &http.Client{Timeout: timeout},
		token:   token,
	}
}

func (u *HTTPUploader) Upload(batch *analyticsv1.UploadBatch) error {
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(batch)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, u.manager+"/api/v1/upload", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if u.token != "" {
		req.Header.Set("X-SysArmor-Agent-Token", u.token)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("upload failed: %s: %s", resp.Status, string(body))
	}
	return nil
}

func normalizeManagerURL(manager string) string {
	if strings.HasPrefix(manager, "http://") || strings.HasPrefix(manager, "https://") {
		return strings.TrimRight(manager, "/")
	}
	return "http://" + strings.TrimRight(manager, "/")
}
