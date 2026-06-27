package detection

import (
	"fmt"
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	"github.com/sysarmor/sysarmor-next-project/internal/sensor/contract"
)

func BenchmarkEngineProcessBuiltinMixed(b *testing.B) {
	engine, _ := New(policymodel.DefaultDetectionPolicy())
	events := benchmarkEventStream(256)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.Process(events[i%len(events)])
	}
	b.ReportMetric(float64(engine.Metrics().CEPRulesScanned)/float64(b.N), "cep_scans/op")
	b.ReportMetric(float64(engine.Metrics().ConditionsEvaluated)/float64(b.N), "conds/op")
}

func BenchmarkEngineProcessCEPRuleCount(b *testing.B) {
	for _, rules := range []int{4, 16, 64, 256} {
		b.Run(fmt.Sprintf("rules_%d", rules), func(b *testing.B) {
			engine, _ := NewWithRuntime(cepPolicy(), contract.CollectionIntent{}, benchmarkCEPContent(rules))
			events := benchmarkEventStream(256)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				engine.Process(events[i%len(events)])
			}
			metrics := engine.Metrics()
			b.ReportMetric(float64(metrics.CEPRulesScanned)/float64(b.N), "cep_scans/op")
			b.ReportMetric(float64(metrics.CEPRulesEvaluated)/float64(b.N), "cep_eval/op")
			b.ReportMetric(float64(metrics.ConditionsEvaluated)/float64(b.N), "conds/op")
		})
	}
}

func BenchmarkEngineProcessContentPrefixCount(b *testing.B) {
	for _, prefixes := range []int{4, 16, 64, 256, 1024} {
		b.Run(fmt.Sprintf("prefixes_%d", prefixes), func(b *testing.B) {
			content := ContentSnapshot{
				ContextRefs: map[string]ContentRef{
					"ctx:bench-prefixes": {Ref: "ctx:bench-prefixes", Version: "v1", Values: benchmarkPrefixes(prefixes)},
				},
				Rules: []RuleSpec{{
					RuleID:            "bench_prefix_file_read",
					Version:           1,
					RuleSetRef:        "ruleset:cep",
					Severity:          "medium",
					RuntimeType:       "expr",
					RequiredBehaviors: []string{"file.read"},
					Expr: ExprSpec{Conditions: []ConditionSpec{
						{Field: "file.path", Op: "prefix", Ref: "ctx:bench-prefixes"},
					}},
				}},
			}
			engine, _ := NewWithRuntime(cepPolicy(), contract.CollectionIntent{}, content)
			events := benchmarkFileReadEvents(256)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				engine.Process(events[i%len(events)])
			}
			metrics := engine.Metrics()
			b.ReportMetric(float64(metrics.CEPRulesScanned)/float64(b.N), "cep_scans/op")
			b.ReportMetric(float64(metrics.ConditionsEvaluated)/float64(b.N), "conds/op")
		})
	}
}

func benchmarkCEPContent(ruleCount int) ContentSnapshot {
	rules := make([]RuleSpec, 0, ruleCount)
	behaviors := []string{"process.exec", "file.write", "file.read", "network.connect"}
	for i := 0; i < ruleCount; i++ {
		behavior := behaviors[i%len(behaviors)]
		field := "process.binary"
		value := "/usr/bin/bench-no-hit"
		switch behavior {
		case "file.write", "file.read":
			field = "file.path"
			value = "/bench/no-hit/"
		case "network.connect":
			field = "socket.port"
			value = "65535"
		}
		rules = append(rules, RuleSpec{
			RuleID:            fmt.Sprintf("bench_rule_%03d", i),
			Version:           1,
			RuleSetRef:        "ruleset:cep",
			Severity:          "low",
			RuntimeType:       "expr",
			RequiredBehaviors: []string{behavior},
			Expr: ExprSpec{Conditions: []ConditionSpec{
				{Field: field, Op: "eq", Value: value},
			}},
		})
	}
	return ContentSnapshot{Rules: rules}
}

func benchmarkEventStream(n int) []*eventv1.CanonicalEvent {
	events := make([]*eventv1.CanonicalEvent, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("bench-%d", i)
		lineage := fmt.Sprintf("lin-%d", i%32)
		switch i % 4 {
		case 0:
			events = append(events, execEvent(id, lineage, fmt.Sprintf("p-%d", i), "parent", "/usr/bin/curl", []string{"/usr/bin/curl", "-fsS", "http://example.invalid"}))
		case 1:
			events = append(events, writeEvent(id, lineage, fmt.Sprintf("p-%d", i), "/usr/bin/curl", fmt.Sprintf("/tmp/bench-%d", i)))
		case 2:
			events = append(events, openEvent(id, lineage, fmt.Sprintf("p-%d", i), "/bin/cat", fmt.Sprintf("/var/log/app/%d.log", i)))
			events[len(events)-1].Behavior = "file.read"
		default:
			events = append(events, connectEventWithParent(id, lineage, fmt.Sprintf("p-%d", i), "parent", "/usr/bin/curl", "198.51.100.10:80"))
		}
	}
	return events
}

func benchmarkFileReadEvents(n int) []*eventv1.CanonicalEvent {
	events := make([]*eventv1.CanonicalEvent, 0, n)
	for i := 0; i < n; i++ {
		ev := openEvent(fmt.Sprintf("read-%d", i), fmt.Sprintf("lin-%d", i%32), fmt.Sprintf("p-%d", i), "/bin/cat", fmt.Sprintf("/bench/path/%04d/secret", i%128))
		ev.Behavior = "file.read"
		events = append(events, ev)
	}
	return events
}

func benchmarkPrefixes(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf("/bench/path/%04d/", i))
	}
	return out
}
