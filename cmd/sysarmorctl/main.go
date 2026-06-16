package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var version = "dev"

func main() {
	mgr := flag.String("mgr", "127.0.0.1:9443", "sysarmor-manager address")
	jsonOut := flag.Bool("json", false, "emit JSON")
	flag.Parse()

	args := flag.Args()
	if len(args) == 1 && args[0] == "version" {
		fmt.Println(version)
		return
	}

	if len(args) == 0 {
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"manager": *mgr})
			return
		}
		fmt.Fprintf(os.Stdout, "sysarmorctl: manager=%s\n", *mgr)
		return
	}

	body, err := query(*mgr, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sysarmorctl: %v\n", err)
		os.Exit(1)
	}
	if *jsonOut {
		_, _ = os.Stdout.Write(body)
		if len(body) == 0 || body[len(body)-1] != '\n' {
			fmt.Println()
		}
		return
	}
	fmt.Println(string(body))
}

func query(mgr string, args []string) ([]byte, error) {
	base := normalizeManagerURL(mgr)
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
	case "link1-sessions":
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
		return httpGet(base + "/api/v1/link1-sessions?" + q.Encode())
	case "link1-downlink":
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
		return httpGet(base + "/api/v1/link1-downlink?" + q.Encode())
	case "link1-resume":
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
		return httpGet(base + "/api/v1/link1-resume?" + q.Encode())
	case "evidence-pullbacks":
		q := url.Values{}
		req := map[string]any{}
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
			case "--scenario":
				i++
				if i < len(args) {
					req["scenario"] = args[i]
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
		if create {
			return httpPostJSON(base+"/api/v1/evidence-pullbacks", req)
		}
		return httpGet(base + "/api/v1/evidence-pullbacks?" + q.Encode())
	case "link1-frames":
		var file string
		for i := 1; i < len(args); i++ {
			if args[i] == "--file" {
				i++
				if i < len(args) {
					file = args[i]
				}
			}
		}
		if file == "" {
			return nil, fmt.Errorf("--file is required")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		return httpPostRaw(base+"/api/v1/link1-frames", data)
	case "metrics":
		return httpGet(base + "/api/v1/metrics")
	case "store-status":
		return httpGet(base + "/api/v1/store-status")
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
	case "events":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--scenario":
				i++
				if i < len(args) {
					q.Set("scenario", args[i])
				}
			case "--kind":
				i++
				if i < len(args) {
					q.Set("kind", args[i])
				}
			case "--limit":
				i++
				if i < len(args) {
					q.Set("limit", args[i])
				}
			case "--offset":
				i++
				if i < len(args) {
					q.Set("offset", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/events?" + q.Encode())
	case "signals":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--scenario":
				i++
				if i < len(args) {
					q.Set("scenario", args[i])
				}
			case "--layer":
				i++
				if i < len(args) {
					q.Set("layer", args[i])
				}
			case "--terminal":
				q.Set("terminal", "true")
			case "--limit":
				i++
				if i < len(args) {
					q.Set("limit", args[i])
				}
			case "--offset":
				i++
				if i < len(args) {
					q.Set("offset", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/signals?" + q.Encode())
	case "incidents":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--scenario":
				i++
				if i < len(args) {
					q.Set("scenario", args[i])
				}
			case "--limit":
				i++
				if i < len(args) {
					q.Set("limit", args[i])
				}
			case "--offset":
				i++
				if i < len(args) {
					q.Set("offset", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/incidents?" + q.Encode())
	case "incident-evidence":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--scenario":
				i++
				if i < len(args) {
					q.Set("scenario", args[i])
				}
			case "--incident-id":
				i++
				if i < len(args) {
					q.Set("incident_id", args[i])
				}
			case "--path-from":
				i++
				if i < len(args) {
					q.Set("path_from", args[i])
				}
			case "--path-to":
				i++
				if i < len(args) {
					q.Set("path_to", args[i])
				}
			case "--seed":
				i++
				if i < len(args) {
					q.Set("seed", args[i])
				}
			case "--hops":
				i++
				if i < len(args) {
					q.Set("hops", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/incident-evidence?" + q.Encode())
	case "incident-evidence-attach":
		req := map[string]any{}
		node := map[string]string{}
		edge := map[string]string{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--scenario":
				i++
				if i < len(args) {
					req["scenario"] = args[i]
				}
			case "--incident-id":
				i++
				if i < len(args) {
					req["incident_id"] = args[i]
				}
			case "--node-id":
				i++
				if i < len(args) {
					node["id"] = args[i]
				}
			case "--node-kind":
				i++
				if i < len(args) {
					node["kind"] = args[i]
				}
			case "--node-label":
				i++
				if i < len(args) {
					node["label"] = args[i]
				}
			case "--edge-id":
				i++
				if i < len(args) {
					edge["id"] = args[i]
				}
			case "--edge-from":
				i++
				if i < len(args) {
					edge["from"] = args[i]
				}
			case "--edge-to":
				i++
				if i < len(args) {
					edge["to"] = args[i]
				}
			case "--edge-kind":
				i++
				if i < len(args) {
					edge["kind"] = args[i]
				}
			}
		}
		evidence := map[string]any{}
		if len(node) > 0 {
			evidence["nodes"] = []map[string]string{node}
		}
		if len(edge) > 0 {
			evidence["edges"] = []map[string]string{edge}
		}
		req["evidence"] = evidence
		return httpPostJSON(base+"/api/v1/incident-evidence", req)
	case "incident-lifecycle":
		req := map[string]string{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--scenario":
				i++
				if i < len(args) {
					req["scenario"] = args[i]
				}
			case "--incident-id":
				i++
				if i < len(args) {
					req["incident_id"] = args[i]
				}
			case "--status":
				i++
				if i < len(args) {
					req["status"] = args[i]
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
		return httpPostJSON(base+"/api/v1/incident-lifecycle", req)
	case "incident-merge":
		req := map[string]string{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--target-incident-id":
				i++
				if i < len(args) {
					req["target_incident_id"] = args[i]
				}
			case "--source-incident-id":
				i++
				if i < len(args) {
					req["source_incident_id"] = args[i]
				}
			}
		}
		return httpPostJSON(base+"/api/v1/incident-merge", req)
	case "recompute":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--scenario":
				i++
				if i < len(args) {
					q.Set("scenario", args[i])
				}
			case "--disable":
				i++
				if i < len(args) {
					q.Set("disable", args[i])
				}
			case "--mode":
				i++
				if i < len(args) {
					q.Set("mode", args[i])
				}
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
		return httpGet(base + "/api/v1/recompute?" + q.Encode())
	case "status":
		return httpGet(base + "/healthz")
	default:
		return nil, fmt.Errorf("unknown command %q", args[0])
	}
}

func normalizeManagerURL(mgr string) string {
	if strings.HasPrefix(mgr, "http://") || strings.HasPrefix(mgr, "https://") {
		return strings.TrimRight(mgr, "/")
	}
	return "http://" + strings.TrimRight(mgr, "/")
}

func httpGet(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	addAuthHeaders(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: %s: %s", url, resp.Status, string(body))
	}
	return body, nil
}

func httpPostJSON(url string, body any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return httpPostRaw(url, data)
}

func httpPostRaw(url string, data []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	addAuthHeaders(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s: %s: %s", url, resp.Status, string(out))
	}
	return out, nil
}

func addAuthHeaders(req *http.Request) {
	if token := os.Getenv("SYSARMOR_DEV_TOKEN"); token != "" {
		req.Header.Set("X-SysArmor-Agent-Token", token)
	}
	if token := os.Getenv("SYSARMOR_OPERATOR_TOKEN"); token != "" {
		req.Header.Set("X-SysArmor-Operator-Token", token)
	}
	if actor := os.Getenv("SYSARMOR_ACTOR"); actor != "" {
		req.Header.Set("X-SysArmor-Actor", actor)
	}
}
