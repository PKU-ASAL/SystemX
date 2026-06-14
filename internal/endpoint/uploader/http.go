package uploader

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type HTTPUploader struct {
	manager string
	client  *http.Client
}

func NewHTTPUploader(manager string) *HTTPUploader {
	return &HTTPUploader{
		manager: normalizeManagerURL(manager),
		client:  http.DefaultClient,
	}
}

func (u *HTTPUploader) Upload(batch *analyticsv1.UploadBatch) error {
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(batch)
	if err != nil {
		return err
	}
	resp, err := u.client.Post(u.manager+"/api/v1/upload", "application/json", bytes.NewReader(data))
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
