package ingest

import (
	"testing"

	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

func TestAnalyzeFilelessC2ProducesCloudSignalsAndIncident(t *testing.T) {
	engine := NewEngine()
	result := engine.Analyze(nil, []*signalv1.Signal{
		endpoint("web_runtime_spawns_shell", "lin-a", false, process("p-web")),
		endpoint("payload_dropped", "lin-a", false, file("/dev/shm/x.sh")),
		endpoint("reverse_shell_pattern", "lin-a", true, process("p-bash"), socket("10.66.0.99:443")),
	})
	if !hasCloud(result.CloudSignals, "dropped_payload_executed_and_connects") || !hasCloud(result.CloudSignals, "web_shell_chain") {
		t.Fatalf("missing cloud signals: %#v", result.CloudSignals)
	}
	if len(result.Incidents) != 1 {
		t.Fatalf("incident count = %d, want 1", len(result.Incidents))
	}
	if result.Incidents[0].GetConverge().GetMethod() != "rarity+causal-topk" {
		t.Fatalf("unexpected converge method %q", result.Incidents[0].GetConverge().GetMethod())
	}
}

func TestAnalyzeStagedDropRequiresCrossLineage(t *testing.T) {
	engine := NewEngine()
	result := engine.Analyze(nil, []*signalv1.Signal{
		endpoint("payload_dropped", "lin-a", false, file("/var/lib/app/plugins/helper")),
		endpoint("suspicious_exec_connect", "lin-b", false, file("/var/lib/app/plugins/helper"), socket("10.66.0.99:443")),
	})
	if len(result.Incidents) != 1 {
		t.Fatalf("incident count = %d, want 1", len(result.Incidents))
	}
	if len(result.Incidents[0].GetLineageIds()) < 2 {
		t.Fatalf("lineage ids = %v, want at least 2", result.Incidents[0].GetLineageIds())
	}
	if !result.CloudSignals[0].GetCrossLineage() {
		t.Fatal("cloud signal should be cross-lineage")
	}
}

func TestAnalyzeStagedDropDisabledCrossLineageSuppressesIncident(t *testing.T) {
	engine := NewEngine()
	result := engine.AnalyzeWithPolicy(nil, []*signalv1.Signal{
		endpoint("payload_dropped", "lin-a", false, file("/var/lib/app/plugins/helper")),
		endpoint("suspicious_exec_connect", "lin-b", false, file("/var/lib/app/plugins/helper"), socket("10.66.0.99:443")),
	}, &policyv1.DetectionPolicy{Converge: &policyv1.ConvergeParams{CrossLineage: false}})
	if len(result.Incidents) != 0 {
		t.Fatalf("incident count = %d, want 0", len(result.Incidents))
	}
}

func TestAnalyzeBenignNoiseProducesNoIncident(t *testing.T) {
	engine := NewEngine()
	result := engine.Analyze(nil, []*signalv1.Signal{
		endpoint("download_by_lolbin", "lin-ci", false, socket("10.66.0.99:8080")),
		endpoint("payload_dropped", "lin-ci", false, file("/tmp/artifact")),
	})
	if len(result.Incidents) != 0 {
		t.Fatalf("incident count = %d, want 0", len(result.Incidents))
	}
}

func TestAnalyzeBenignNoiseAdditiveControlProducesIncident(t *testing.T) {
	engine := NewEngine()
	result := engine.AnalyzeWithPolicy(nil, []*signalv1.Signal{
		endpoint("download_by_lolbin", "lin-ci", false, socket("10.66.0.99:8080")),
		endpoint("download_by_lolbin", "lin-ci", false, socket("10.66.0.99:8080")),
	}, &policyv1.DetectionPolicy{Converge: &policyv1.ConvergeParams{Mode: "additive_threshold", AdditiveRiskThreshold: 100}})
	if len(result.Incidents) != 1 {
		t.Fatalf("incident count = %d, want 1", len(result.Incidents))
	}
}

func endpoint(name, lineage string, terminal bool, entities ...*signalv1.EntityRef) *signalv1.Signal {
	return &signalv1.Signal{
		Name:         name,
		Where:        signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		BaseRisk:     50,
		GlobalRarity: 1,
		LineageId:    lineage,
		Terminal:     terminal,
		Entities:     entities,
		Scenario:     "scenario-a",
	}
}

func process(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "process", Key: key, Role: "subject"}
}

func file(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "file", Key: "file:" + key, Role: "object"}
}

func socket(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "socket", Key: "socket:" + key, Role: "object"}
}

func hasCloud(signals []*signalv1.Signal, name string) bool {
	for _, sig := range signals {
		if sig.GetName() == name {
			return true
		}
	}
	return false
}
