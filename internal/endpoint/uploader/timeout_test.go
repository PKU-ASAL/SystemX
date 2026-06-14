package uploader

import (
	"testing"
	"time"
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
