package kafka

import "testing"

func TestNewWriterProducerRequiresBrokers(t *testing.T) {
	if _, err := NewWriterProducer(nil); err == nil {
		t.Fatal("NewWriterProducer(nil) error = nil, want disabled")
	}
}
