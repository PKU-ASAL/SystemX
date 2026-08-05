package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

var version = "dev"

func main() {
	managerURL := flag.String("manager-url", defaultManagerURL(), "sysarmor-manager HTTP URL")
	socketPath := flag.String("socket", defaultAgentSock(), "local sysarmor-agent control socket")
	jsonOut := flag.Bool("json", false, "emit JSON")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		usage()
		return
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Println(version)
		return
	}
	if len(args) == 0 {
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"manager_url": *managerURL, "socket": *socketPath})
			return
		}
		usage()
		return
	}

	if len(args) >= 2 && args[0] == "content" && args[1] == "diff" {
		body, err := contentDiff(args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sysarmorctl: %v\n", err)
			os.Exit(1)
		}
		_, _ = os.Stdout.Write(body)
		if len(body) == 0 || body[len(body)-1] != '\n' {
			fmt.Println()
		}
		return
	}

	if isLocalAgentCommand(args) {
		if isStreamingWatchCommand(args) {
			if err := streamLocalAgent(*socketPath, args); err != nil {
				fmt.Fprintf(os.Stderr, "sysarmorctl: %v\n", err)
				os.Exit(1)
			}
			return
		}
		body, err := queryLocalAgentWithManager(*socketPath, *managerURL, args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sysarmorctl: %v\n", err)
			os.Exit(1)
		}
		_, _ = os.Stdout.Write(body)
		if len(body) == 0 || body[len(body)-1] != '\n' {
			fmt.Println()
		}
		return
	}

	body, err := query(*managerURL, args)
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

func usage() {
	fmt.Fprint(os.Stdout, `Usage:
  sysarmorctl [--socket PATH] [--json] agent health
  sysarmorctl [--socket PATH] policy current
  sysarmorctl [--socket PATH] policy apply --file policy.json
  sysarmorctl [--socket PATH] content apply --file content.json --allow-unsigned
  sysarmorctl [--socket PATH] debug profile cpu --seconds 10 --output agent.cpu.pb.gz
  sysarmorctl [--socket PATH] event watch --include-recent --limit 10
  sysarmorctl [--socket PATH] signal watch --include-events --limit 10
  sysarmorctl [--socket PATH] enroll --manager-url URL (--token TOKEN | --token-file PATH) [--upload-history] [--timeout DURATION]
  sysarmorctl [--socket PATH] unenroll [--timeout DURATION]

  sysarmorctl [--manager-url URL] manager agents list
  sysarmorctl [--manager-url URL] manager policies assign --agent AGENT --policy-id POLICY --version N [--downlink]
  sysarmorctl [--manager-url URL] manager control-commands list --agent AGENT
  sysarmorctl [--manager-url URL] manager control-commands create content --agent AGENT --file content.json
  sysarmorctl [--manager-url URL] manager control-commands cancel --command-id ID --agent AGENT
  sysarmorctl [--manager-url URL] manager artifacts upload --file agent.tar.gz --name sysarmor-agent --kind agent --version v1 --os linux --arch amd64
  sysarmorctl [--manager-url URL] manager artifacts list [--kind agent] [--status active]
  sysarmorctl [--manager-url URL] manager channels upsert --channel stable --artifact-id ARTIFACT
  sysarmorctl [--manager-url URL] manager channels list
  sysarmorctl [--manager-url URL] manager enrollments list
  sysarmorctl [--manager-url URL] manager enrollments create --agent-id AGENT --gateway-addr HOST:PORT [--channel stable] [--artifact-id ARTIFACT] [--ttl 24h]
  sysarmorctl [--manager-url URL] manager evidence pullbacks --create --agent-id AGENT --incident-id ID --label key=value

Global flags:
  --socket PATH        local agent Unix socket, default $SYSARMOR_AGENT_SOCK or /run/sysarmor/agent/control.sock
  --manager-url URL    manager HTTP URL, default $SYSARMOR_MANAGER_URL or http://127.0.0.1:9443
  --json               emit JSON without extra formatting

Local commands stay at the top level. Manager HTTP administration must use the manager namespace.
`)
}

func defaultManagerURL() string {
	if v := strings.TrimSpace(os.Getenv("SYSARMOR_MANAGER_URL")); v != "" {
		return v
	}
	return "http://127.0.0.1:9443"
}
