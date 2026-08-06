package main

import (
	"fmt"
	"strings"
)

func query(mgr string, args []string) ([]byte, error) {
	base := normalizeManagerURL(mgr)
	if len(args) > 0 && args[0] == "manager" {
		return queryManager(base, args[1:])
	}
	return nil, fmt.Errorf("unknown command %q; manager HTTP commands must use the manager namespace", strings.Join(args, " "))
}

func queryManagerAPI(base string, args []string) ([]byte, error) {
	switch args[0] {
	case "agents", "agent-health", "agent-sessions", "enrollments", "data-resume":
		return queryManagerIdentityAPI(base, args)
	case "rules", "policies", "policy-publish", "policy-audit", "policy-assignments", "effective-policy":
		return queryManagerPolicyAPI(base, args)
	case "evidence-pullbacks", "responses", "response-decision", "response-approval":
		return queryManagerControlAPI(base, args)
	case "metrics", "store-status", "rarity-baseline", "events", "signals", "recompute":
		return queryManagerTelemetryAPI(base, args)
	case "incidents", "incident-evidence", "incident-evidence-attach", "incident-lifecycle", "incident-merge":
		return queryManagerIncidentAPI(base, args)
	case "artifacts":
		return queryManagerArtifactsAPI(base, args[1:])
	case "channels":
		return queryManagerChannelsAPI(base, args[1:])
	case "status":
		return httpGet(base + "/healthz")
	default:
		return nil, fmt.Errorf("unknown command %q", args[0])
	}
}

func queryManager(base string, args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("manager command is required")
	}
	switch args[0] {
	case "agents":
		return queryManagerAPI(base, append([]string{"agents"}, managerArgsAfterAction(args, "list")...))
	case "health":
		return queryManagerAPI(base, append([]string{"agent-health"}, managerArgsAfterAction(args, "list", "get")...))
	case "sessions":
		return queryManagerAPI(base, append([]string{"agent-sessions"}, managerArgsAfterAction(args, "list")...))
	case "artifacts":
		return queryManagerArtifacts(base, args[1:])
	case "channels":
		return queryManagerChannels(base, args[1:])
	case "enrollments":
		return queryManagerEnrollments(base, args[1:])
	case "resume":
		return queryManagerAPI(base, append([]string{"data-resume"}, managerArgsAfterAction(args, "get")...))
	case "metrics":
		return queryManagerAPI(base, []string{"metrics"})
	case "status":
		return queryManagerAPI(base, []string{"status"})
	case "store":
		if len(args) >= 2 && args[1] == "status" {
			return queryManagerAPI(base, []string{"store-status"})
		}
	case "policies":
		return queryManagerPolicies(base, args[1:])
	case "control-commands":
		return queryManagerControlCommands(base, args[1:])
	case "responses":
		return queryManagerAPI(base, append([]string{"responses"}, managerArgsAfterAction(args, "list")...))
	case "response":
		if len(args) >= 2 && args[1] == "decide" {
			return queryManagerAPI(base, append([]string{"response-decision"}, args[2:]...))
		}
		if len(args) >= 2 && args[1] == "approve" {
			return queryManagerAPI(base, append([]string{"response-approval"}, args[2:]...))
		}
	case "incidents":
		return queryManagerAPI(base, append([]string{"incidents"}, managerArgsAfterAction(args, "list")...))
	case "incident":
		if len(args) >= 3 && args[1] == "evidence" && args[2] == "attach" {
			return queryManagerAPI(base, append([]string{"incident-evidence-attach"}, args[3:]...))
		}
		if len(args) >= 2 && args[1] == "evidence" {
			return queryManagerAPI(base, append([]string{"incident-evidence"}, args[2:]...))
		}
		if len(args) >= 2 && args[1] == "lifecycle" {
			return queryManagerAPI(base, append([]string{"incident-lifecycle"}, args[2:]...))
		}
		if len(args) >= 2 && args[1] == "merge" {
			return queryManagerAPI(base, append([]string{"incident-merge"}, args[2:]...))
		}
	case "evidence":
		if len(args) >= 2 && args[1] == "pullbacks" {
			return queryManagerAPI(base, append([]string{"evidence-pullbacks"}, args[2:]...))
		}
	case "events":
		return queryManagerAPI(base, append([]string{"events"}, managerArgsAfterAction(args, "list")...))
	case "signals":
		return queryManagerAPI(base, append([]string{"signals"}, managerArgsAfterAction(args, "list")...))
	case "rarity":
		if len(args) >= 2 && args[1] == "baseline" {
			return queryManagerAPI(base, append([]string{"rarity-baseline"}, args[2:]...))
		}
	case "rules":
		return queryManagerAPI(base, append([]string{"rules"}, managerArgsAfterAction(args, "list")...))
	case "recompute":
		return queryManagerAPI(base, args)
	}
	return nil, fmt.Errorf("unknown manager command %q", strings.Join(args, " "))
}

func queryManagerEnrollments(base string, args []string) ([]byte, error) {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list":
		return queryManagerAPI(base, append([]string{"enrollments"}, managerArgsAfterAction(args, "list")...))
	case "create":
		return queryManagerAPI(base, append([]string{"enrollments", "--create"}, args[1:]...))
	default:
		return nil, fmt.Errorf("unknown manager enrollments command %q", args[0])
	}
}

func managerArgsAfterAction(args []string, actions ...string) []string {
	if len(args) >= 2 {
		for _, action := range actions {
			if args[1] == action {
				return args[2:]
			}
		}
	}
	if len(args) <= 1 {
		return nil
	}
	return args[1:]
}

func normalizeManagerURL(mgr string) string {
	if strings.HasPrefix(mgr, "http://") || strings.HasPrefix(mgr, "https://") {
		return strings.TrimRight(mgr, "/")
	}
	return "http://" + strings.TrimRight(mgr, "/")
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
