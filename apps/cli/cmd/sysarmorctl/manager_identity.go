package main

import (
	"fmt"
	"net/url"
)

func queryManagerIdentityAPI(base string, args []string) ([]byte, error) {
	switch args[0] {
	case "agents":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--tenant-id":
				i++
				if i < len(args) {
					q.Set("tenant_id", args[i])
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
			case "--health-status":
				i++
				if i < len(args) {
					q.Set("health_status", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/agents?" + q.Encode())
	case "agent-health":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--agent-id":
				i++
				if i < len(args) {
					q.Set("agent_id", args[i])
				}
			case "--tenant-id":
				i++
				if i < len(args) {
					q.Set("tenant_id", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/agent-health?" + q.Encode())
	case "agent-sessions":
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
		return httpGet(base + "/api/v1/agent-sessions?" + q.Encode())
	case "enrollments":
		q := url.Values{}
		req := map[string]any{}
		labels := map[string]string{}
		create := false
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--create":
				create = true
			case "--tenant-id":
				i++
				if i < len(args) {
					q.Set("tenant_id", args[i])
					req["tenant_id"] = args[i]
				}
			case "--agent-id", "--agent":
				i++
				if i < len(args) {
					req["agent_id"] = args[i]
				}
			case "--host-id":
				i++
				if i < len(args) {
					req["host_id"] = args[i]
				}
			case "--gateway-addr":
				i++
				if i < len(args) {
					req["gateway_addr"] = args[i]
				}
			case "--gateway-sni":
				i++
				if i < len(args) {
					req["gateway_sni"] = args[i]
				}
			case "--profile":
				i++
				if i < len(args) {
					req["profile"] = args[i]
				}
			case "--channel":
				i++
				if i < len(args) {
					req["channel"] = args[i]
				}
			case "--artifact-id":
				i++
				if i < len(args) {
					req["artifact_id"] = args[i]
				}
			case "--artifact-url":
				i++
				if i < len(args) {
					req["artifact_url"] = args[i]
				}
			case "--ttl":
				i++
				if i < len(args) {
					req["ttl"] = args[i]
				}
			case "--label":
				i++
				if i < len(args) {
					addLabelMap(labels, args[i])
				}
			case "--actor":
				i++
				if i < len(args) {
					req["actor"] = args[i]
				}
			case "--status":
				i++
				if i < len(args) {
					q.Set("status", args[i])
				}
			}
		}
		if len(labels) > 0 {
			req["labels"] = labels
		}
		if create {
			return httpPostJSON(base+"/api/v1/enrollments", req)
		}
		return httpGet(base + "/api/v1/enrollments?" + q.Encode())
	case "data-resume":
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
		return httpGet(base + "/api/v1/data-resume?" + q.Encode())
	default:
		return nil, fmt.Errorf("unknown command %q", args[0])
	}
}
