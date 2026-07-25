package detection

import (
	"fmt"
	"strings"
)

func validateRuleSpecs(rules []effectiveRule) []string {
	var out []string
	for _, rule := range rules {
		out = append(out, validateRuleSpec(rule.spec)...)
	}
	return out
}

func validateRuleSpec(rule RuleSpec) []string {
	switch ruleRuntimeType(rule) {
	case "expr":
		return validateConditions(rule.RuleID, "expr", rule.Expr.Conditions, nil)
	case "sequence":
		return validateSequence(rule)
	default:
		return nil
	}
}

func validateSequence(rule RuleSpec) []string {
	var out []string
	for _, field := range rule.Sequence.By {
		if compileField(field) == fieldUnknown {
			out = append(out, fmt.Sprintf("rule %s sequence has unsupported by field %q", rule.RuleID, field))
		}
	}
	prior := make(map[string]bool, len(rule.Sequence.Steps))
	for _, step := range rule.Sequence.Steps {
		id := strings.TrimSpace(step.ID)
		if prior[id] {
			out = append(out, fmt.Sprintf("rule %s sequence has duplicate step %q", rule.RuleID, id))
		}
		out = append(out, validateConditions(rule.RuleID, "step "+id, step.Conditions, prior)...)
		prior[id] = true
	}
	return out
}

func validateConditions(ruleID, location string, conditions []ConditionSpec, prior map[string]bool) []string {
	var out []string
	for _, condition := range conditions {
		if compileField(condition.Field) == fieldUnknown {
			out = append(out, fmt.Sprintf("rule %s %s has unsupported field %q", ruleID, location, condition.Field))
		}
		if compileOp(condition.Op) == 0 {
			out = append(out, fmt.Sprintf("rule %s %s has unsupported operator %q", ruleID, location, condition.Op))
			continue
		}
		if compileOp(condition.Op) != opSameAs {
			continue
		}
		step := strings.TrimSpace(condition.Step)
		if prior == nil || step == "" || !prior[step] {
			out = append(out, fmt.Sprintf("rule %s %s same_as references unknown prior step %q", ruleID, location, step))
		}
		if compileField(firstNonEmpty(condition.StepField, condition.Field)) == fieldUnknown {
			out = append(out, fmt.Sprintf("rule %s %s has unsupported step field %q", ruleID, location, condition.StepField))
		}
	}
	return out
}

func ruleRuntimeType(rule RuleSpec) string {
	if rule.RuntimeType != "" {
		return strings.ToLower(strings.TrimSpace(rule.RuntimeType))
	}
	return strings.ToLower(strings.TrimSpace(rule.Runtime))
}
