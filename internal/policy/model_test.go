package policy

import (
	"encoding/json"
	"testing"
)

func TestUnifiedPolicyUsesTelemetrySection(t *testing.T) {
	policy := DefaultPolicy("default")
	policy.Telemetry = &TelemetryPolicy{MaxBatchItems: 256, MaxBatchBytes: 262144, FlushInterval: "1s"}
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document["telemetry"]) == 0 {
		t.Fatalf("unified policy has no telemetry section: %s", raw)
	}
	if _, legacy := document["data_plane"]; legacy {
		t.Fatalf("unified policy contains legacy data_plane section: %s", raw)
	}
}

func TestTelemetryPolicyContainsOnlyBatchSettings(t *testing.T) {
	raw, err := json.Marshal(TelemetryPolicy{MaxBatchItems: 512, MaxBatchBytes: 524288, FlushInterval: "2s"})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"max_batch_items":512,"max_batch_bytes":524288,"flush_interval":"2s"}` {
		t.Fatalf("telemetry policy JSON = %s", raw)
	}
}

func TestDefaultDetectionPolicyContainsNoBuiltinContentRefs(t *testing.T) {
	policy := DefaultDetectionPolicy()
	if len(policy.RuleSets) != 0 || len(policy.ContextRefs) != 0 || len(policy.IOCRefs) != 0 || len(policy.RuleOverrides) != 0 {
		t.Fatalf("default detection policy still embeds content refs: %+v", policy)
	}
}
