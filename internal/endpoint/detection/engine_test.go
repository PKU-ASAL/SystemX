package detection

import (
	"strings"
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

func TestBuiltinRuleSetEmitsMultiEventPayloadLifecycle(t *testing.T) {
	engine, report := New(policymodel.DefaultDetectionPolicy())
	if report.Status != "applied" {
		t.Fatalf("report = %+v", report)
	}
	events := []*eventv1.CanonicalEvent{
		writeEvent("e1", "lin-a", "p1", "/usr/bin/curl", "/var/lib/app/plugins/helper"),
		chmodEvent("e2", "lin-a", "p2", "/usr/bin/chmod", "/var/lib/app/plugins/helper"),
		execEvent("e3", "lin-a", "helper-stable", "", "/var/lib/app/plugins/helper", nil),
		connectEventWithParent("e4", "lin-a", "child", "helper-stable", "/bin/bash", "10.66.0.99:443"),
	}
	var lifecycleRefs []string
	var lifecycleRuleVersion uint64
	var lifecycleRuleSet string
	for _, ev := range events {
		for _, sig := range engine.Process(ev) {
			if sig.GetName() == "payload_lifecycle" {
				lifecycleRefs = sig.GetEventRefs()
				lifecycleRuleVersion = sig.GetRuleVersion()
				lifecycleRuleSet = sig.GetRulesetRef()
			}
		}
	}
	for _, want := range []string{"e1", "e2", "e3", "e4"} {
		if !contains(lifecycleRefs, want) {
			t.Fatalf("payload_lifecycle refs = %v, want %s", lifecycleRefs, want)
		}
	}
	if lifecycleRuleVersion != 1 || lifecycleRuleSet != "ruleset:endpoint-linux-builtin" {
		t.Fatalf("payload_lifecycle rule metadata version=%d ruleset=%q", lifecycleRuleVersion, lifecycleRuleSet)
	}
}

func TestRuleOverrideDisablesBuiltinRule(t *testing.T) {
	disabled := false
	policy := policymodel.DefaultDetectionPolicy()
	policy.RuleOverrides = append(policy.RuleOverrides, policymodel.RuleOverride{RuleID: "payload_dropped", Enabled: &disabled})
	engine, _ := New(policy)
	signals := engine.Process(writeEvent("e1", "lin-a", "p1", "/usr/bin/curl", "/dev/shm/x.sh"))
	for _, sig := range signals {
		if sig.GetName() == "payload_dropped" {
			t.Fatalf("payload_dropped emitted despite override: %+v", sig)
		}
	}
}

func TestDependencyCheckReportsMissingCollectionInput(t *testing.T) {
	policy := policymodel.DefaultDetectionPolicy()
	_, report := NewWithInputs(policy, contract.CollectionIntent{Behaviors: []string{"process.exec"}})
	if report.Status != "degraded" || len(report.Warnings) == 0 {
		t.Fatalf("report = %+v, want degraded with warnings", report)
	}
}

func TestDependencyCheckReportsMissingFields(t *testing.T) {
	enabled := true
	policy := &policymodel.DetectionPolicy{
		PolicyID: "field-dependency-test",
		Version:  1,
		Mode:     "observe",
		RuleSets: []policymodel.RuleSetRef{{Ref: "ruleset:field", Enabled: &enabled}},
	}
	content := ContentSnapshot{Rules: []RuleSpec{{
		RuleID:      "needs_socket_port",
		Version:     1,
		RuleSetRef:  "ruleset:field",
		Severity:    "medium",
		RuntimeType: "expr",
		Expr:        ExprSpec{Conditions: []ConditionSpec{{Field: "socket.port", Op: "eq", Value: "443"}}},
		RequiredEvents: []RequiredEventSpec{{
			Behavior: "network.connect",
			Fields:   []string{"socket.port", "process.binary"},
		}},
		RequiredBehaviors: []string{"network.connect"},
	}}}
	_, report := NewWithRuntime(policy, contract.CollectionIntent{Behaviors: []string{"process.exec"}}, content)
	if report.Status != "degraded" || !containsWarning(report.Warnings, "socket.port") {
		t.Fatalf("report = %+v, want missing socket.port", report)
	}
	_, report = NewWithRuntime(policy, contract.CollectionIntent{Behaviors: []string{"network.connect"}}, content)
	if report.Status != "applied" {
		t.Fatalf("report = %+v, want applied", report)
	}
}

func TestDependencyCheckUsesCollectionCapabilityFields(t *testing.T) {
	enabled := true
	policy := &policymodel.DetectionPolicy{
		PolicyID: "capability-field-test",
		Version:  1,
		Mode:     "observe",
		RuleSets: []policymodel.RuleSetRef{{Ref: "ruleset:field", Enabled: &enabled}},
	}
	content := ContentSnapshot{Rules: []RuleSpec{{
		RuleID:      "needs_socket_port",
		Version:     1,
		RuleSetRef:  "ruleset:field",
		Severity:    "medium",
		RuntimeType: "expr",
		Expr:        ExprSpec{Conditions: []ConditionSpec{{Field: "socket.port", Op: "eq", Value: "443"}}},
		RequiredEvents: []RequiredEventSpec{{
			Behavior: "network.connect",
			Fields:   []string{"socket.port"},
		}},
		RequiredBehaviors: []string{"network.connect"},
	}}}
	_, report := NewWithRuntime(policy, contract.CollectionIntent{
		Behaviors: []string{"network.connect"},
		Capabilities: []contract.CollectionBehaviorCapability{{
			Behavior: "network.connect",
			Fields:   []string{"process.binary"},
		}},
	}, content)
	if report.Status != "degraded" || !containsWarning(report.Warnings, "socket.port") {
		t.Fatalf("report = %+v, want capability missing socket.port", report)
	}
}

func TestSignalCarriesContextAndIOCRefs(t *testing.T) {
	engine, _ := New(policymodel.DefaultDetectionPolicy())
	var gotContext bool
	var gotIOC bool
	for _, sig := range engine.Process(writeEvent("e1", "lin-a", "p1", "/usr/bin/curl", "/dev/shm/x.sh")) {
		if sig.GetName() != "payload_dropped" {
			continue
		}
		for _, ref := range sig.GetContextRefs() {
			gotContext = gotContext || ref.GetRef() == "ctx:payload-path-prefixes"
		}
	}
	for _, sig := range engine.Process(connectEventWithParent("e2", "lin-a", "p2", "parent", "/bin/bash", "10.66.0.99:443")) {
		if sig.GetName() != "reverse_shell_pattern" {
			continue
		}
		for _, ref := range sig.GetIocRefs() {
			gotIOC = gotIOC || ref.GetRef() == "ioc:c2-port-feed"
		}
	}
	if !gotContext || !gotIOC {
		t.Fatalf("context=%t ioc=%t, want both refs on emitted signals", gotContext, gotIOC)
	}
}

func TestCredentialReadSuppressesDuplicateProcessPathSignals(t *testing.T) {
	enabled := true
	policy := policymodel.DefaultDetectionPolicy()
	policy.RuleOverrides = append(policy.RuleOverrides, policymodel.RuleOverride{RuleID: "credential_file_read", Enabled: &enabled})
	engine, _ := New(policy)
	first := openEvent("e1", "lin-a", "proc-a", "/tmp/cat", "/root/.ssh/id_rsa")
	second := openEvent("e2", "lin-a", "proc-a", "/tmp/cat", "/root/.ssh/id_rsa")
	if got := countSignals(engine.Process(first), "credential_file_read"); got != 1 {
		t.Fatalf("first credential signal count = %d, want 1", got)
	}
	if got := countSignals(engine.Process(second), "credential_file_read"); got != 0 {
		t.Fatalf("duplicate credential signal count = %d, want 0", got)
	}
	third := openEvent("e3", "lin-a", "proc-a", "/tmp/cat", "/run/secrets/token")
	if got := countSignals(engine.Process(third), "credential_file_read"); got != 1 {
		t.Fatalf("different path credential signal count = %d, want 1", got)
	}
}

func countSignals(signals []*signalv1.Signal, name string) int {
	count := 0
	for _, sig := range signals {
		if sig.GetName() == name {
			count++
		}
	}
	return count
}

func TestRuntimeContentSnapshotOverridesIOC(t *testing.T) {
	policy := policymodel.DefaultDetectionPolicy()
	engine, _ := NewWithRuntime(policy, contract.CollectionIntent{}, ContentSnapshot{
		IOCRefs: map[string]ContentRef{
			"ioc:c2-port-feed": {
				Ref:     "ioc:c2-port-feed",
				Version: "local-test",
				Digest:  "digest-a",
				Values:  []string{"9443"},
			},
		},
	})
	if signals := engine.Process(connectEventWithParent("e1", "lin-a", "p1", "parent", "/bin/bash", "10.66.0.99:443")); len(signals) != 0 {
		t.Fatalf("443 signals = %+v, want none after IOC override", signals)
	}
	var gotVersion string
	for _, sig := range engine.Process(connectEventWithParent("e2", "lin-a", "p1", "parent", "/bin/bash", "10.66.0.99:9443")) {
		if sig.GetName() != "reverse_shell_pattern" {
			continue
		}
		for _, ref := range sig.GetIocRefs() {
			if ref.GetRef() == "ioc:c2-port-feed" {
				gotVersion = ref.GetVersion()
			}
		}
	}
	if gotVersion != "local-test" {
		t.Fatalf("ioc ref version = %q, want local-test", gotVersion)
	}
}

func TestRuntimeRulePackMetadataOverridesBuiltinRule(t *testing.T) {
	enabled := true
	policy := &policymodel.DetectionPolicy{
		PolicyID: "rulepack-test",
		Version:  1,
		Mode:     "observe",
		RuleSets: []policymodel.RuleSetRef{{Ref: "ruleset:test", Version: "v1", Enabled: &enabled}},
		IOCRefs:  []policymodel.ContentRef{{Ref: "ioc:c2-port-feed", Version: "builtin"}},
	}
	engine, _ := NewWithRuntime(policy, contract.CollectionIntent{}, ContentSnapshot{
		Rules: []RuleSpec{{
			RuleID:            "reverse_shell_pattern",
			Version:           7,
			RuleSetRef:        "ruleset:test",
			Severity:          "critical",
			Runtime:           "builtin.reverse_shell_pattern",
			RequiredBehaviors: []string{"network.connect"},
			IOCRefs:           []string{"ioc:c2-port-feed"},
		}},
	})
	for _, sig := range engine.Process(connectEventWithParent("e1", "lin-a", "p1", "parent", "/bin/bash", "10.66.0.99:443")) {
		if sig.GetName() == "reverse_shell_pattern" && sig.GetRuleVersion() == 7 && sig.GetRulesetRef() == "ruleset:test" {
			return
		}
	}
	t.Fatal("reverse_shell_pattern from external ruleset not emitted")
}

func TestCEPRuntimeExprRuleUsesContentRef(t *testing.T) {
	enabled := true
	policy := &policymodel.DetectionPolicy{
		PolicyID:    "cep-expr-test",
		Version:     1,
		Mode:        "observe",
		RuleSets:    []policymodel.RuleSetRef{{Ref: "ruleset:cep", Enabled: &enabled}},
		ContextRefs: []policymodel.ContentRef{{Ref: "ctx:credential-path-prefixes", Version: "v1"}},
	}
	engine, report := NewWithRuntime(policy, contract.CollectionIntent{}, ContentSnapshot{
		ContextRefs: map[string]ContentRef{
			"ctx:credential-path-prefixes": {Ref: "ctx:credential-path-prefixes", Version: "v1", Values: []string{"/run/secrets/"}},
		},
		Rules: []RuleSpec{{
			RuleID:      "cep_credential_read",
			Version:     3,
			RuleSetRef:  "ruleset:cep",
			Severity:    "high",
			RuntimeType: "expr",
			Expr: ExprSpec{Conditions: []ConditionSpec{
				{Field: "file.path", Op: "prefix", Ref: "ctx:credential-path-prefixes"},
			}},
			RequiredBehaviors: []string{"file.open"},
			ContextRefs:       []string{"ctx:credential-path-prefixes"},
		}},
	})
	if report.Status != "applied" {
		t.Fatalf("report = %+v", report)
	}
	signals := engine.Process(openEvent("e1", "lin-a", "p1", "/bin/cat", "/run/secrets/token"))
	if len(signals) != 1 || signals[0].GetName() != "cep_credential_read" {
		t.Fatalf("signals = %+v", signals)
	}
	if signals[0].GetRuleVersion() != 3 || signals[0].GetContextRefs()[0].GetVersion() != "v1" {
		t.Fatalf("signal metadata = %+v", signals[0])
	}
}

func TestCEPRuntimeSequenceRuleEmitsMultipleEventRefs(t *testing.T) {
	enabled := true
	policy := &policymodel.DetectionPolicy{
		PolicyID: "cep-sequence-test",
		Version:  1,
		Mode:     "observe",
		RuleSets: []policymodel.RuleSetRef{{Ref: "ruleset:cep", Enabled: &enabled}},
	}
	engine, _ := NewWithRuntime(policy, contract.CollectionIntent{}, ContentSnapshot{
		Rules: []RuleSpec{{
			RuleID:      "cep_payload_lifecycle",
			Version:     2,
			RuleSetRef:  "ruleset:cep",
			Severity:    "critical",
			RuntimeType: "sequence",
			Sequence: SequenceSpec{
				Within: 60 * 1_000_000_000,
				By:     []string{"lineage_id"},
				Steps: []StepSpec{
					{ID: "drop", Behavior: "file.write", Conditions: []ConditionSpec{{Field: "file.path", Op: "prefix", Value: "/dev/shm/"}}},
					{ID: "chmod", Behavior: "file.chmod", Conditions: []ConditionSpec{{Field: "file.path", Op: "same_as", Step: "drop"}}},
					{ID: "exec", Behavior: "process.exec", Conditions: []ConditionSpec{{Field: "process.binary", Op: "same_as", Step: "drop", StepField: "file.path"}}},
					{ID: "connect", Behavior: "network.connect", Conditions: []ConditionSpec{{Field: "socket.port", Op: "in", Values: []string{"443"}}}},
				},
			},
			RequiredBehaviors: []string{"file.write", "file.chmod", "process.exec", "network.connect"},
		}},
	})
	events := []*eventv1.CanonicalEvent{
		writeEventAt("e1", "lin-a", "p1", "/usr/bin/curl", "/dev/shm/x", 1),
		chmodEventAt("e2", "lin-a", "p2", "/usr/bin/chmod", "/dev/shm/x", 2),
		execEventAt("e3", "lin-a", "p3", "", "/dev/shm/x", nil, 3),
		connectEventAt("e4", "lin-a", "p3", "", "/dev/shm/x", "10.0.0.1:443", 4),
	}
	var got []*signalv1.Signal
	for _, ev := range events {
		got = append(got, engine.Process(ev)...)
	}
	var refs []string
	for _, sig := range got {
		if sig.GetName() == "cep_payload_lifecycle" {
			refs = sig.GetEventRefs()
		}
	}
	for _, want := range []string{"e1", "e2", "e3", "e4"} {
		if !contains(refs, want) {
			t.Fatalf("cep refs = %v, want %s; signals=%+v", refs, want, got)
		}
	}
}

func TestCEPRuntimeSequenceExpiresWindow(t *testing.T) {
	engine, _ := NewWithRuntimeLimits(cepPolicy(), contract.CollectionIntent{}, ContentSnapshot{Rules: []RuleSpec{cepSequenceRule()}}, EngineLimits{})
	if signals := engine.Process(writeEventAt("e1", "lin-a", "p1", "/bin/curl", "/dev/shm/x", 1)); len(signals) != 0 {
		t.Fatalf("signals after first step = %+v", signals)
	}
	if signals := engine.Process(chmodEventAt("e2", "lin-a", "p2", "/bin/chmod", "/dev/shm/x", uint64(120*1_000_000_000))); len(signals) != 0 {
		t.Fatalf("signals after expired step = %+v", signals)
	}
	metrics := engine.Metrics()
	if metrics.ExpiredCEPGroups == 0 {
		t.Fatalf("metrics = %+v, want expired group", metrics)
	}
}

func TestCEPRuntimeEvictsGroupsAtLimit(t *testing.T) {
	engine, _ := NewWithRuntimeLimits(cepPolicy(), contract.CollectionIntent{}, ContentSnapshot{Rules: []RuleSpec{cepSequenceRule()}}, EngineLimits{MaxCEPGroups: 2, MaxCEPRefs: 8})
	for _, lineage := range []string{"lin-a", "lin-b", "lin-c"} {
		engine.Process(writeEventAt("drop-"+lineage, lineage, "p1", "/bin/curl", "/dev/shm/"+lineage, 1))
	}
	metrics := engine.Metrics()
	if metrics.EvictedCEPGroups == 0 || metrics.ActiveCEPGroups > 2 {
		t.Fatalf("metrics = %+v, want eviction and active <= 2", metrics)
	}
}

func TestCEPRuntimeDropsRefsAtLimit(t *testing.T) {
	engine, _ := NewWithRuntimeLimits(cepPolicy(), contract.CollectionIntent{}, ContentSnapshot{Rules: []RuleSpec{cepSequenceRule()}}, EngineLimits{MaxCEPGroups: 8, MaxCEPRefs: 2})
	events := []*eventv1.CanonicalEvent{
		writeEventAt("e1", "lin-a", "p1", "/bin/curl", "/dev/shm/x", 1),
		chmodEventAt("e2", "lin-a", "p2", "/bin/chmod", "/dev/shm/x", 2),
		execEventAt("e3", "lin-a", "p3", "", "/dev/shm/x", nil, 3),
		connectEventAt("e4", "lin-a", "p3", "", "/dev/shm/x", "10.0.0.1:443", 4),
	}
	var refs []string
	for _, ev := range events {
		for _, sig := range engine.Process(ev) {
			if sig.GetName() == "cep_payload_lifecycle" {
				refs = sig.GetEventRefs()
			}
		}
	}
	metrics := engine.Metrics()
	if len(refs) != 2 || metrics.DroppedEventRefs == 0 {
		t.Fatalf("refs=%v metrics=%+v, want 2 refs and dropped refs", refs, metrics)
	}
}

func execEvent(id, lineage, stable, parent, bin string, argv []string) *eventv1.CanonicalEvent {
	return &eventv1.CanonicalEvent{
		Id: id, Behavior: "process.exec", LineageId: lineage, ParentStableId: parent,
		SubjectProc: &eventv1.ProcessRef{StableId: stable, Binary: bin, Argv: argv},
	}
}

func connectEventWithParent(id, lineage, stable, parent, bin, dst string) *eventv1.CanonicalEvent {
	return &eventv1.CanonicalEvent{
		Id: id, Behavior: "network.connect", LineageId: lineage, ParentStableId: parent,
		SubjectProc: &eventv1.ProcessRef{StableId: stable, Binary: bin},
		Object:      &eventv1.ObjectRef{Kind: "socket", SocketAddr: dst},
	}
}

func writeEvent(id, lineage, stable, bin, file string) *eventv1.CanonicalEvent {
	return &eventv1.CanonicalEvent{
		Id: id, Behavior: "file.write", LineageId: lineage,
		SubjectProc: &eventv1.ProcessRef{StableId: stable, Binary: bin},
		Object:      &eventv1.ObjectRef{Kind: "file", FilePath: file},
	}
}

func chmodEvent(id, lineage, stable, bin, file string) *eventv1.CanonicalEvent {
	return &eventv1.CanonicalEvent{
		Id: id, Behavior: "file.chmod", LineageId: lineage,
		SubjectProc: &eventv1.ProcessRef{StableId: stable, Binary: bin},
		Object:      &eventv1.ObjectRef{Kind: "file", FilePath: file},
	}
}

func openEvent(id, lineage, stable, bin, file string) *eventv1.CanonicalEvent {
	return &eventv1.CanonicalEvent{
		Id: id, Behavior: "file.open", LineageId: lineage,
		SubjectProc: &eventv1.ProcessRef{StableId: stable, Binary: bin},
		Object:      &eventv1.ObjectRef{Kind: "file", FilePath: file},
	}
}

func writeEventAt(id, lineage, stable, bin, file string, ts uint64) *eventv1.CanonicalEvent {
	ev := writeEvent(id, lineage, stable, bin, file)
	ev.OccurredAtNs = ts
	return ev
}

func chmodEventAt(id, lineage, stable, bin, file string, ts uint64) *eventv1.CanonicalEvent {
	ev := chmodEvent(id, lineage, stable, bin, file)
	ev.OccurredAtNs = ts
	return ev
}

func execEventAt(id, lineage, stable, parent, bin string, argv []string, ts uint64) *eventv1.CanonicalEvent {
	ev := execEvent(id, lineage, stable, parent, bin, argv)
	ev.OccurredAtNs = ts
	return ev
}

func connectEventAt(id, lineage, stable, parent, bin, dst string, ts uint64) *eventv1.CanonicalEvent {
	ev := connectEventWithParent(id, lineage, stable, parent, bin, dst)
	ev.OccurredAtNs = ts
	return ev
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsWarning(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}

func cepPolicy() *policymodel.DetectionPolicy {
	enabled := true
	return &policymodel.DetectionPolicy{
		PolicyID: "cep-test",
		Version:  1,
		Mode:     "observe",
		RuleSets: []policymodel.RuleSetRef{{Ref: "ruleset:cep", Enabled: &enabled}},
	}
}

func cepSequenceRule() RuleSpec {
	return RuleSpec{
		RuleID:      "cep_payload_lifecycle",
		Version:     2,
		RuleSetRef:  "ruleset:cep",
		Severity:    "critical",
		RuntimeType: "sequence",
		Sequence: SequenceSpec{
			Within: 60 * 1_000_000_000,
			By:     []string{"lineage_id"},
			Steps: []StepSpec{
				{ID: "drop", Behavior: "file.write", Conditions: []ConditionSpec{{Field: "file.path", Op: "prefix", Value: "/dev/shm/"}}},
				{ID: "chmod", Behavior: "file.chmod", Conditions: []ConditionSpec{{Field: "file.path", Op: "same_as", Step: "drop"}}},
				{ID: "exec", Behavior: "process.exec", Conditions: []ConditionSpec{{Field: "process.binary", Op: "same_as", Step: "drop", StepField: "file.path"}}},
				{ID: "connect", Behavior: "network.connect", Conditions: []ConditionSpec{{Field: "socket.port", Op: "in", Values: []string{"443"}}}},
			},
		},
	}
}
