package detection

import (
	"fmt"
	"strings"
	"time"
)

const maxSuppressionWindow = 24 * time.Hour
const maxConditionTreeDepth = 8
const maxConditionTreeLeaves = 256

func validateRuleSpecs(rules []effectiveRule) []string {
	var out []string
	for _, rule := range rules {
		out = append(out, validateRuleSpec(rule.spec)...)
	}
	return out
}

func validateRuleSpec(rule RuleSpec) []string {
	out := validateSuppression(rule)
	switch ruleRuntimeType(rule) {
	case "expr":
		out = append(out, validateConditions(rule.RuleID, "expr", rule.Expr.Conditions, nil)...)
		return append(out, validateConditionTree(rule.RuleID, "expr", rule.Expr.ConditionGroup, nil)...)
	case "sequence":
		return append(out, validateSequence(rule)...)
	default:
		return out
	}
}

func validateSuppression(rule RuleSpec) []string {
	suppression := rule.Suppression
	if suppression.Within == 0 && len(suppression.By) == 0 {
		return nil
	}
	var out []string
	if ruleRuntimeType(rule) != "expr" {
		out = append(out, fmt.Sprintf("rule %s suppression requires expr runtime", rule.RuleID))
	}
	if suppression.Within <= 0 || suppression.Within > maxSuppressionWindow {
		out = append(out, fmt.Sprintf("rule %s suppression window must be between 0 and %s", rule.RuleID, maxSuppressionWindow))
	}
	if len(suppression.By) == 0 {
		out = append(out, fmt.Sprintf("rule %s suppression by field is required", rule.RuleID))
	}
	for _, field := range suppression.By {
		if compileField(field) == fieldUnknown {
			out = append(out, fmt.Sprintf("rule %s has unsupported suppression by field %q", rule.RuleID, field))
		}
	}
	return out
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
		out = append(out, validateConditionTree(rule.RuleID, "step "+id, step.ConditionGroup, prior)...)
		prior[id] = true
	}
	return out
}

func validateConditionTree(ruleID, location string, node *ConditionNodeSpec, prior map[string]bool) []string {
	if node == nil {
		return nil
	}
	leaves := 0
	return validateConditionNode(ruleID, location, node, prior, 1, &leaves)
}

func validateConditionNode(ruleID, location string, node *ConditionNodeSpec, prior map[string]bool, depth int, leaves *int) []string {
	if depth > maxConditionTreeDepth {
		return []string{fmt.Sprintf("rule %s %s condition tree exceeds maximum depth %d", ruleID, location, maxConditionTreeDepth)}
	}
	kinds := boolCount(node.Condition != nil, node.Not != nil, node.All != nil, node.Any != nil)
	if kinds != 1 {
		return []string{fmt.Sprintf("rule %s %s condition node must set exactly one kind", ruleID, location)}
	}
	if node.Condition != nil {
		*leaves++
		if *leaves > maxConditionTreeLeaves {
			return []string{fmt.Sprintf("rule %s %s condition tree exceeds maximum leaves %d", ruleID, location, maxConditionTreeLeaves)}
		}
		return validateConditions(ruleID, location, []ConditionSpec{*node.Condition}, prior)
	}
	if node.Not != nil {
		return validateConditionNode(ruleID, location, node.Not, prior, depth+1, leaves)
	}
	children := node.All
	group := "all"
	if node.Any != nil {
		children = node.Any
		group = "any"
	}
	if len(children) == 0 {
		return []string{fmt.Sprintf("rule %s %s %s requires children", ruleID, location, group)}
	}
	var out []string
	for i := range children {
		out = append(out, validateConditionNode(ruleID, location, &children[i], prior, depth+1, leaves)...)
	}
	return out
}

func boolCount(values ...bool) int {
	count := 0
	for _, value := range values {
		if value {
			count++
		}
	}
	return count
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
