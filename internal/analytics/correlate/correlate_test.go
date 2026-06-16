package correlate

import (
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	policyv1 "github.com/sysarmor/sysarmor-next-project/api/proto/policy/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

func TestBuildFiltersEndpointRulesAndFindsScenario(t *testing.T) {
	view := Build([]*eventv1.CanonicalEvent{{Scenario: "event-scenario"}}, []*signalv1.Signal{
		{Name: "payload_dropped", Scenario: "signal-scenario", Entities: []*signalv1.EntityRef{{Kind: "file", Key: "/tmp/x", Role: "object"}}},
		{Name: "reverse_shell_pattern", Terminal: true},
	}, &policyv1.DetectionPolicy{EndpointRules: []string{"reverse_shell_pattern"}})
	if view.Scenario != "signal-scenario" {
		t.Fatalf("scenario = %q", view.Scenario)
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
