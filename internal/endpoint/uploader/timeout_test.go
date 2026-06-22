package uploader

import (
	"testing"
	"time"
)

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

func TestGRPCUploaderStoresAgentToken(t *testing.T) {
	up := NewGRPCUploaderWithOptions("127.0.0.1:9443", time.Second, "dev-token")
	if up.token != "dev-token" {
		t.Fatalf("token = %q", up.token)
	}
}
