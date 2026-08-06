package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
)

func queryManagerControlCommands(base string, args []string) ([]byte, error) {
	if len(args) == 0 {
		args = []string{"list"}
	}
	switch args[0] {
	case "list":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--tenant", "--tenant-id":
				i++
				if i < len(args) {
					q.Set("tenant_id", args[i])
				}
			case "--agent", "--agent-id":
				i++
				if i < len(args) {
					q.Set("agent_id", args[i])
				}
			case "--type":
				i++
				if i < len(args) {
					q.Set("type", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/control-commands?" + q.Encode())
	case "create":
		return managerControlCommandCreate(base, args[1:])
	case "cancel", "retry", "expire":
		return managerControlCommandAction(base, args[0], args[1:])
	default:
		return nil, fmt.Errorf("unknown manager control-commands command %q", args[0])
	}
}

func managerControlCommandAction(base, action string, args []string) ([]byte, error) {
	req := map[string]any{"action": action}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--command-id":
			i++
			if i < len(args) {
				req["command_id"] = args[i]
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
	return httpPostJSON(base+"/api/v1/control-commands", req)
}

func managerControlCommandCreate(base string, args []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("control command type is required")
	}
	commandKind := args[0]
	req := map[string]any{}
	switch commandKind {
	case "content":
		req["type"] = "content_update"
	case "policy":
		req["type"] = "policy_update"
	default:
		return nil, fmt.Errorf("unsupported control command create type %q", commandKind)
	}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--command-id":
			i++
			if i < len(args) {
				req["command_id"] = args[i]
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
		case "--file":
			i++
			if i < len(args) {
				raw, err := os.ReadFile(args[i])
				if err != nil {
					return nil, err
				}
				req["payload_json"] = json.RawMessage(raw)
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
	return httpPostJSON(base+"/api/v1/control-commands", req)
}

func queryManagerControlAPI(base string, args []string) ([]byte, error) {
	switch args[0] {
	case "evidence-pullbacks":
		q := url.Values{}
		req := map[string]any{}
		labels := map[string]string{}
		create := false
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--create":
				create = true
			case "--request-id":
				i++
				if i < len(args) {
					req["request_id"] = args[i]
				}
			case "--tenant-id":
				i++
				if i < len(args) {
					q.Set("tenant_id", args[i])
					req["tenant_id"] = args[i]
				}
			case "--agent-id":
				i++
				if i < len(args) {
					q.Set("agent_id", args[i])
					req["agent_id"] = args[i]
				}
			case "--incident-id":
				i++
				if i < len(args) {
					req["incident_id"] = args[i]
				}
			case "--label":
				i++
				if i < len(args) {
					addLabelMap(labels, args[i])
				}
			case "--target":
				i++
				if i < len(args) {
					req["target"] = args[i]
				}
			case "--reason":
				i++
				if i < len(args) {
					req["reason"] = args[i]
				}
			case "--actor":
				i++
				if i < len(args) {
					req["actor"] = args[i]
				}
			}
		}
		if len(labels) > 0 {
			req["labels"] = labels
		}
		if create {
			return httpPostJSON(base+"/api/v1/evidence-pullbacks", req)
		}
		return httpGet(base + "/api/v1/evidence-pullbacks?" + q.Encode())
	case "responses":
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
			case "--pending":
				q.Set("pending", "true")
			}
		}
		return httpGet(base + "/api/v1/responses?" + q.Encode())
	case "response-decision":
		req := map[string]string{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--signal-id":
				i++
				if i < len(args) {
					req["signal_id"] = args[i]
				}
			case "--tenant-id":
				i++
				if i < len(args) {
					req["tenant_id"] = args[i]
				}
			case "--agent-id":
				i++
				if i < len(args) {
					req["agent_id"] = args[i]
				}
			case "--actor":
				i++
				if i < len(args) {
					req["actor"] = args[i]
				}
			case "--target":
				i++
				if i < len(args) {
					req["target"] = args[i]
				}
			}
		}
		return httpPostJSON(base+"/api/v1/response-decisions", req)
	case "response-approval":
		req := map[string]any{"approved": true}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--response-id":
				i++
				if i < len(args) {
					req["response_id"] = args[i]
				}
			case "--tenant-id":
				i++
				if i < len(args) {
					req["tenant_id"] = args[i]
				}
			case "--agent-id":
				i++
				if i < len(args) {
					req["agent_id"] = args[i]
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
			case "--reject":
				req["approved"] = false
			}
		}
		return httpPostJSON(base+"/api/v1/response-approvals", req)
	default:
		return nil, fmt.Errorf("unknown command %q", args[0])
	}
}
