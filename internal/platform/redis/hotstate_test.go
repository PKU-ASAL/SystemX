package redis

import (
	"context"
	"testing"
)

func TestNoopHotStateAcceptsSession(t *testing.T) {
	if err := (NoopHotState{}).TouchAgentSession(context.Background(), AgentSession{TenantID: "default", AgentID: "agent-a"}); err != nil {
		t.Fatalf("TouchAgentSession() error = %v", err)
	}
}

func TestNewClientHotStateRequiresAddress(t *testing.T) {
	if _, err := NewClientHotState("", 0); err == nil {
		t.Fatal("NewClientHotState empty addr error = nil, want disabled")
	}
}
