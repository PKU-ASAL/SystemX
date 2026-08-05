package correlate

import (
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/event/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/signal/v1"
)

func TestBuildFiltersEndpointRulesAndFindsCommonLabels(t *testing.T) {
	labels := map[string]string{"case_type": "scenario", "scenario": "apt-staged-drop"}
	view := Build([]*eventv1.CanonicalEvent{{Labels: labels}}, []*signalv1.Signal{
		{Name: "payload_dropped", Labels: labels, Entities: []*signalv1.EntityRef{{Kind: "file", Key: "/tmp/x", Role: "object"}}},
		{Name: "reverse_shell_pattern", Terminal: true, Labels: labels},
	}, &policyv1.DetectionPolicy{EndpointRules: []string{"reverse_shell_pattern"}})
	if view.Labels["scenario"] != "apt-staged-drop" || view.Labels["case_type"] != "scenario" {
		t.Fatalf("labels = %#v", view.Labels)
	}
	if view.Has("payload_dropped") {
		t.Fatal("disabled rule is present")
	}
	if !view.HasTerminal("reverse_shell_pattern") {
		t.Fatal("terminal reverse shell missing")
	}
}

func TestCollectEntitiesNormalizesAndDeduplicates(t *testing.T) {
	view := Build(nil, []*signalv1.Signal{
		{Name: "payload_dropped", Entities: []*signalv1.EntityRef{
			{Kind: "file", Key: "/tmp/x", Role: "object"},
			{Kind: "file", Key: "file:/tmp/x", Role: "object"},
		}},
	}, nil)
	got := view.CollectEntities("payload_dropped")
	if len(got) != 1 || got[0].GetKey() != "file:/tmp/x" {
		t.Fatalf("entities = %#v", got)
	}
}
