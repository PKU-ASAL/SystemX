package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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
		return httpGet(base + "/api/v1/agents")
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
	case "metrics":
		return httpGet(base + "/api/v1/metrics")
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
			}
		}
		return httpGet(base + "/api/v1/signals?" + q.Encode())
	case "incidents":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			if args[i] == "--scenario" {
				i++
				if i < len(args) {
					q.Set("scenario", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/incidents?" + q.Encode())
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
	resp, err := http.Get(url)
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
