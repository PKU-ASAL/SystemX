package incident

import (
	"testing"

	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/analytics/converge"
	"github.com/sysarmor/sysarmor-next-project/apps/manager/internal/analytics/rarity"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/incident/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestIncidentReportDescriptorDefinesFormalIdentity(t *testing.T) {
	descriptor := (&incidentv1.Incident{}).ProtoReflect().Descriptor()
	for _, name := range []string{"tenant_id", "correlation_key", "analysis_version", "first_observed_at", "last_observed_at"} {
		if descriptor.Fields().ByName(protoreflect.Name(name)) == nil {
			t.Fatalf("Incident field %s is missing", name)
		}
	}
}

func TestBuilderCreatesIncidentWithEvidence(t *testing.T) {
	builder := NewBuilder()
	inc := builder.Build([]*signalv1.Signal{
		{
			Name:         "reverse_shell_pattern",
			BaseRisk:     80,
			GlobalRarity: 1,
			LineageId:    "lin-a",
			Terminal:     true,
			Labels:       map[string]string{"case_type": "scenario", "scenario": "scenario-a"},
			Entities: []*signalv1.EntityRef{
				{Kind: "process", Key: "process:p-bash", Role: "subject"},
				{Kind: "socket", Key: "socket:10.66.0.99:443", Role: "object"},
			},
		},
	}, converge.Decision{Incident: true, Method: "rarity+causal-topk", Controls: []string{"terminal_reverse_shell"}})
	if inc.GetConverge().GetScore() != 80 || inc.GetConverge().GetMethod() != "rarity+causal-topk" {
		t.Fatalf("converge = %+v", inc.GetConverge())
	}
	if len(inc.GetEvidence().GetEdges()) == 0 {
		t.Fatalf("evidence edges missing: %+v", inc.GetEvidence())
	}
	if len(inc.GetTerminals()) != 1 || inc.GetTerminals()[0] != "process:p-bash" {
		t.Fatalf("terminals = %v", inc.GetTerminals())
	}
	if inc.GetLabels()["scenario"] != "scenario-a" {
		t.Fatalf("labels = %#v", inc.GetLabels())
	}
}

func TestBuilderCanUseWorkloadBaselineScorer(t *testing.T) {
	builder := &Builder{Scorer: rarity.WorkloadBaselineScorer{Baseline: rarity.Baseline{WorkloadCounts: map[string]map[string]uint64{
		"container:checkout-api": {"reverse_shell_pattern": 3},
	}}}}
	inc := builder.Build([]*signalv1.Signal{
		{
			Name:         "reverse_shell_pattern",
			BaseRisk:     80,
			GlobalRarity: 1,
			Terminal:     true,
			Entities: []*signalv1.EntityRef{
				{Kind: "container", Key: "checkout-api"},
				{Kind: "process", Key: "process:p-bash", Role: "subject"},
			},
		},
	}, converge.Decision{Incident: true, Method: "rarity+causal-topk", Controls: []string{"terminal_reverse_shell"}})
	if inc.GetConverge().GetScore() != 20 {
		t.Fatalf("score = %f, want 20", inc.GetConverge().GetScore())
	}
}
