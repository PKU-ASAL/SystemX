package main

import (
	"fmt"
	"net/url"
	"strconv"
)

func queryManagerPolicies(base string, args []string) ([]byte, error) {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list", "get":
		return queryManagerAPI(base, append([]string{"policies"}, args[1:]...))
	case "publish":
		return queryManagerAPI(base, append([]string{"policy-publish"}, args[1:]...))
	case "audit":
		return queryManagerAPI(base, append([]string{"policy-audit"}, args[1:]...))
	case "assignments":
		return queryManagerAPI(base, append([]string{"policy-assignments"}, managerArgsAfterAction(args, "list")...))
	case "assign":
		return managerPolicyAssign(base, args[1:])
	case "effective":
		return queryManagerAPI(base, append([]string{"effective-policy"}, args[1:]...))
	default:
		return nil, fmt.Errorf("unknown manager policies command %q", args[0])
	}
}

func managerPolicyAssign(base string, args []string) ([]byte, error) {
	req := map[string]any{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--assignment-id":
			i++
			if i < len(args) {
				req["assignment_id"] = args[i]
			}
		case "--tenant", "--tenant-id":
			i++
			if i < len(args) {
				req["tenant_id"] = args[i]
			}
		case "--agent", "--agent-id":
			i++
			if i < len(args) {
				req["agent_id"] = args[i]
			}
		case "--scope-type":
			i++
			if i < len(args) {
				req["scope_type"] = args[i]
			}
		case "--scope-selector":
			i++
			if i < len(args) {
				req["scope_selector"] = args[i]
			}
		case "--policy-id":
			i++
			if i < len(args) {
				req["policy_id"] = args[i]
			}
		case "--version", "--policy-version":
			i++
			if i < len(args) {
				version, err := strconv.ParseUint(args[i], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid --version: %w", err)
				}
				req["policy_version"] = version
			}
		case "--downlink":
			req["downlink"] = true
		case "--command-id":
			i++
			if i < len(args) {
				req["command_id"] = args[i]
			}
		case "--actor":
			i++
			if i < len(args) {
				req["actor"] = args[i]
			}
		case "--reason":
			i++
			if i < len(args) {
				req["reason"] = args[i]
			}
		}
	}
	return httpPostJSON(base+"/api/v1/policy-assignments", req)
}

func queryManagerPolicyAPI(base string, args []string) ([]byte, error) {
	switch args[0] {
	case "rules":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			if args[i] == "--where" {
				i++
				if i < len(args) {
					q.Set("where", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/rules?" + q.Encode())
	case "policies":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--tenant-id":
				i++
				if i < len(args) {
					q.Set("tenant_id", args[i])
				}
			case "--policy-id":
				i++
				if i < len(args) {
					q.Set("policy_id", args[i])
				}
			case "--version":
				i++
				if i < len(args) {
					q.Set("version", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/policies?" + q.Encode())
	case "policy-publish":
		req := map[string]any{"published": true}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--tenant-id":
				i++
				if i < len(args) {
					req["tenant_id"] = args[i]
				}
			case "--policy-id":
				i++
				if i < len(args) {
					req["policy_id"] = args[i]
				}
			case "--version":
				i++
				if i < len(args) {
					version, err := strconv.ParseUint(args[i], 10, 64)
					if err != nil {
						return nil, fmt.Errorf("invalid --version: %w", err)
					}
					req["version"] = version
				}
			case "--unpublish":
				req["published"] = false
			case "--actor":
				i++
				if i < len(args) {
					req["actor"] = args[i]
				}
			case "--reason":
				i++
				if i < len(args) {
					req["reason"] = args[i]
				}
			}
		}
		return httpPostJSON(base+"/api/v1/policy-publish", req)
	case "policy-audit":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--tenant-id":
				i++
				if i < len(args) {
					q.Set("tenant_id", args[i])
				}
			case "--policy-id":
				i++
				if i < len(args) {
					q.Set("policy_id", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/policy-audit?" + q.Encode())
	case "policy-assignments":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--tenant-id":
				i++
				if i < len(args) {
					q.Set("tenant_id", args[i])
				}
			case "--agent-id":
				i++
				if i < len(args) {
					q.Set("agent_id", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/policy-assignments?" + q.Encode())
	case "effective-policy":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--tenant-id":
				i++
				if i < len(args) {
					q.Set("tenant_id", args[i])
				}
			case "--agent-id":
				i++
				if i < len(args) {
					q.Set("agent_id", args[i])
				}
			case "--scope-type":
				i++
				if i < len(args) {
					q.Set("scope_type", args[i])
				}
			case "--scope-selector":
				i++
				if i < len(args) {
					q.Set("scope_selector", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/effective-policy?" + q.Encode())
	default:
		return nil, fmt.Errorf("unknown command %q", args[0])
	}
}
