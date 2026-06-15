package uploader

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
)

func TestHTTPUploaderUsesConfiguredTimeout(t *testing.T) {
	up := NewHTTPUploaderWithTimeout("127.0.0.1:9443", 250*time.Millisecond)
	if up.client.Timeout != 250*time.Millisecond {
		t.Fatalf("timeout = %s", up.client.Timeout)
	}
}

func TestHTTPUploaderFallsBackToDefaultTimeout(t *testing.T) {
	up := NewHTTPUploaderWithTimeout("127.0.0.1:9443", 0)
	if up.client.Timeout != 10*time.Second {
		t.Fatalf("timeout = %s", up.client.Timeout)
	}
}

func TestGRPCUploaderUsesConfiguredTimeout(t *testing.T) {
	up := NewGRPCUploaderWithTimeout("127.0.0.1:9443", 250*time.Millisecond)
	if up.timeout != 250*time.Millisecond {
		t.Fatalf("timeout = %s", up.timeout)
	}
}

func TestGRPCUploaderFallsBackToDefaultTimeout(t *testing.T) {
	up := NewGRPCUploaderWithTimeout("127.0.0.1:9443", 0)
	if up.timeout != 10*time.Second {
		t.Fatalf("timeout = %s", up.timeout)
	}
}

func TestHTTPUploaderSendsAgentToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-SysArmor-Agent-Token"); got != "dev-token" {
			t.Fatalf("token header = %q", got)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	up := NewHTTPUploaderWithOptions(server.URL, time.Second, "dev-token")
	if _, err := up.Upload(&analyticsv1.UploadBatch{}); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
}

func TestGRPCUploaderStoresAgentToken(t *testing.T) {
	up := NewGRPCUploaderWithOptions("127.0.0.1:9443", time.Second, "dev-token")
	if up.token != "dev-token" {
		t.Fatalf("token = %q", up.token)
	}
}
