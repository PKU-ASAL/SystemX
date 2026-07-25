package detection

import (
	"time"

	"github.com/sysarmor/sysarmor-next-project/internal/eventmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
)

func builtinRules() []RuleSpec {
	collect := &policymodel.ResponseIntentRef{Action: "collect_evidence", Confidence: 80, Reason: "terminal endpoint signal"}
	nonTerminal := false
	return []RuleSpec{
		{
			RuleID: "web_runtime_spawns_shell", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "high",
			RuntimeType: "sequence", Terminal: &nonTerminal,
			RequiredEvents: []RequiredEventSpec{{Behavior: eventmodel.BehaviorProcessExec.String(), Fields: []string{"process.binary_name", "process.stable_id", "parent.stable_id", "lineage_id"}}},
			ContextRefs:    []string{"ctx:web-runtime-binaries", "ctx:shell-binaries"},
			Sequence: SequenceSpec{Within: 30 * time.Second, By: []string{"lineage_id"}, Steps: []StepSpec{
				{ID: "runtime", Behavior: eventmodel.BehaviorProcessExec.String(), Conditions: []ConditionSpec{{Field: "process.binary_name", Op: "in", Ref: "ctx:web-runtime-binaries"}}},
				{ID: "shell", Behavior: eventmodel.BehaviorProcessExec.String(), Conditions: []ConditionSpec{
					{Field: "process.binary_name", Op: "in", Ref: "ctx:shell-binaries"},
					{Field: "parent.stable_id", Op: "same_as", Step: "runtime", StepField: "process.stable_id"},
				}},
			}},
		},
		{RuleID: "download_by_lolbin", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "medium", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorNetworkConnect.String()}, IOCRefs: []string{"ioc:c2-download-port-feed"}},
		{RuleID: "payload_dropped", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "high", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorFileChmod.String()}, ContextRefs: []string{"ctx:payload-path-prefixes"}},
		{RuleID: "reverse_shell_pattern", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "critical", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorNetworkConnect.String()}, IOCRefs: []string{"ioc:c2-control-port-feed"}, ResponseIntent: collect},
		{RuleID: "suspicious_exec_connect", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "high", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorNetworkConnect.String()}, IOCRefs: []string{"ioc:c2-control-port-feed"}},
		{RuleID: "payload_lifecycle", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "high", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorFileWrite.String(), eventmodel.BehaviorProcessExec.String(), eventmodel.BehaviorNetworkConnect.String()}, ContextRefs: []string{"ctx:payload-path-prefixes"}, IOCRefs: []string{"ioc:c2-control-port-feed"}},
		{RuleID: "credential_file_read", Version: 1, RuleSetRef: builtinRuleSetRef, Where: "endpoint", Severity: "medium", Runtime: "builtin", RequiredBehaviors: []string{eventmodel.BehaviorFileRead.String()}, ContextRefs: []string{"ctx:credential-path-prefixes", "ctx:trusted-admin-binaries"}},
	}
}
