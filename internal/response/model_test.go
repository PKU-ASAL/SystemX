package response

import "testing"

func TestScopeDecision(t *testing.T) {
	tests := []struct {
		name         string
		command      Scope
		runtime      Scope
		runtimeKnown bool
		allowed      bool
		reason       string
	}{
		{
			name:    "empty command scope is allowed",
			allowed: true,
		},
		{
			name:         "matching command scope is allowed",
			command:      Scope{Type: "container", Selector: "abc123"},
			runtime:      Scope{Type: "container", Selector: "abc123"},
			runtimeKnown: true,
			allowed:      true,
		},
		{
			name:         "missing runtime scope denies explicit command scope",
			command:      Scope{Type: "container", Selector: "abc123"},
			runtimeKnown: false,
			reason:       "agent runtime scope is required for scoped response command",
		},
		{
			name:         "mismatched command scope is denied",
			command:      Scope{Type: "container", Selector: "wrong"},
			runtime:      Scope{Type: "container", Selector: "abc123"},
			runtimeKnown: true,
			reason:       "response command scope does not match agent runtime scope",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := ScopeDecision(tt.command, tt.runtime, tt.runtimeKnown)
			if decision.Allowed != tt.allowed {
				t.Fatalf("allowed = %t, want %t", decision.Allowed, tt.allowed)
			}
			if tt.reason != "" && decision.Reason != tt.reason {
				t.Fatalf("reason = %q, want %q", decision.Reason, tt.reason)
			}
		})
	}
}
