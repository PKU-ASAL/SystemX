package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var version = "dev"

func main() {
	mgr := flag.String("mgr", "127.0.0.1:9443", "sysarmor-manager address")
	agentSock := flag.String("agent-sock", defaultAgentSock(), "local sysarmor-agent control socket")
	jsonOut := flag.Bool("json", false, "emit JSON")
	flag.Parse()

	args := flag.Args()
	if len(args) == 1 && args[0] == "version" {
		fmt.Println(version)
		return
	}

	if len(args) == 0 {
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"manager": *mgr, "agent_sock": *agentSock})
			return
		}
		fmt.Fprintf(os.Stdout, "sysarmorctl: manager=%s agent_sock=%s\n", *mgr, *agentSock)
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
		body, err := queryLocalAgent(*agentSock, args)
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

func defaultAgentSock() string {
	if v := strings.TrimSpace(os.Getenv("SYSARMOR_AGENT_SOCK")); v != "" {
		return v
	}
	return "/var/run/sysarmor/agent.sock"
}

func isLocalAgentCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[0] {
	case "agent":
		return args[1] == "health" || args[1] == "capability"
	case "policy":
		return args[1] == "current" || args[1] == "apply" || args[1] == "explain"
	case "content":
		return args[1] == "apply" || args[1] == "list" || args[1] == "get"
	case "event":
		return args[1] == "watch" || args[1] == "get"
	case "signal":
		return args[1] == "watch"
	default:
		return false
	}
}

func queryLocalAgent(socketPath string, args []string) ([]byte, error) {
	if strings.TrimSpace(socketPath) == "" {
		return nil, fmt.Errorf("--agent-sock is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout(args, 5*time.Second))
	defer cancel()
	conn, err := grpc.DialContext(ctx, "unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		}),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := controlv1.NewAgentControlServiceClient(conn)
	reqCtx := requestContext(args)
	switch args[0] + " " + args[1] {
	case "agent health":
		resp, err := client.Health(ctx, &controlv1.HealthRequest{Context: reqCtx})
		if err != nil {
			return nil, err
		}
		return marshalProtoJSON(resp)
	case "agent capability":
		resp, err := client.Capability(ctx, &controlv1.CapabilityRequest{Context: reqCtx})
		if err != nil {
			return nil, err
		}
		return marshalProtoJSON(resp)
	case "policy current":
		resp, err := client.CurrentPolicy(ctx, &controlv1.CurrentPolicyRequest{Context: reqCtx})
		if err != nil {
			return nil, err
		}
		return marshalProtoJSON(resp)
	case "policy apply":
		policyJSON, err := policyPayload(args)
		if err != nil {
			return nil, err
		}
		policyType := flagValue(args, "--type")
		if policyType == "" && len(args) > 2 && args[2] == "collection" {
			policyType = "collection"
		}
		resp, err := client.ApplyPolicy(ctx, &controlv1.ApplyPolicyRequest{
			Context:    reqCtx,
			PolicyType: policyType,
			PolicyJson: policyJSON,
			DryRun:     hasFlag(args, "--dry-run"),
		})
		if err != nil {
			return nil, err
		}
		return marshalProtoJSON(resp)
	case "policy explain":
		policyJSON, err := policyPayload(args)
		if err != nil {
			return nil, err
		}
		policyType := flagValue(args, "--type")
		if policyType == "" && len(args) > 2 && args[2] == "collection" {
			policyType = "collection"
		}
		resp, err := client.ApplyPolicy(ctx, &controlv1.ApplyPolicyRequest{
			Context:    reqCtx,
			PolicyType: policyType,
			PolicyJson: policyJSON,
			DryRun:     true,
		})
		if err != nil {
			return nil, err
		}
		if hasFlag(args, "--report-only") && resp.GetReportJson() != "" {
			return []byte(resp.GetReportJson()), nil
		}
		return marshalProtoJSON(resp)
	case "content apply":
		contentJSON, err := contentPayload(args)
		if err != nil {
			return nil, err
		}
		resp, err := client.ApplyContent(ctx, &controlv1.ApplyContentRequest{
			Context:       reqCtx,
			ContentJson:   contentJSON,
			DryRun:        hasFlag(args, "--dry-run"),
			AllowUnsigned: hasFlag(args, "--allow-unsigned"),
		})
		if err != nil {
			return nil, err
		}
		return marshalProtoJSON(resp)
	case "content list":
		resp, err := client.ListContent(ctx, &controlv1.ListContentRequest{
			Context: reqCtx,
			Kind:    flagValue(args, "--kind"),
		})
		if err != nil {
			return nil, err
		}
		return marshalProtoJSON(resp)
	case "content get":
		resp, err := client.GetContent(ctx, &controlv1.GetContentRequest{
			Context: reqCtx,
			Ref:     flagValue(args, "--ref"),
		})
		if err != nil {
			return nil, err
		}
		return marshalProtoJSON(resp)
	case "event get":
		resp, err := client.GetEvent(ctx, &controlv1.GetEventRequest{
			Context: reqCtx,
			EventId: firstNonEmpty(flagValue(args, "--event-id"), flagValue(args, "--id")),
		})
		if err != nil {
			return nil, err
		}
		return marshalProtoJSON(resp)
	case "event watch":
		stream, err := client.WatchEvents(ctx, &controlv1.WatchEventsRequest{
			Context:       reqCtx,
			Behavior:      flagValue(args, "--behavior"),
			Limit:         uint32Flag(args, "--limit"),
			IncludeRecent: hasFlag(args, "--include-recent"),
			SnapshotOnly:  hasFlag(args, "--snapshot"),
			Filter:        watchFilter(args),
		})
		if err != nil {
			return nil, err
		}
		return collectEventFrames(stream)
	case "signal watch":
		stream, err := client.WatchSignals(ctx, &controlv1.WatchSignalsRequest{
			Context:       reqCtx,
			RuleId:        flagValue(args, "--rule-id"),
			Where:         flagValue(args, "--where"),
			Limit:         uint32Flag(args, "--limit"),
			IncludeRecent: hasFlag(args, "--include-recent"),
			SnapshotOnly:  hasFlag(args, "--snapshot"),
			Filter:        watchFilter(args),
		})
		if err != nil {
			return nil, err
		}
		if hasFlag(args, "--include-events") {
			return collectSignalFramesWithEvents(ctx, client, reqCtx, stream)
		}
		return collectSignalFrames(stream)
	default:
		return nil, fmt.Errorf("unsupported local agent command %q", strings.Join(args, " "))
	}
}

func collectEventFrames(stream controlv1.AgentControlService_WatchEventsClient) ([]byte, error) {
	var out strings.Builder
	for {
		frame, err := stream.Recv()
		if err == io.EOF {
			return []byte(out.String()), nil
		}
		if err != nil {
			if out.Len() > 0 && status.Code(err) == codes.DeadlineExceeded {
				return []byte(out.String()), nil
			}
			return []byte(out.String()), err
		}
		data, err := marshalProtoJSONLine(frame)
		if err != nil {
			return []byte(out.String()), err
		}
		out.Write(data)
		out.WriteByte('\n')
	}
}

func collectSignalFrames(stream controlv1.AgentControlService_WatchSignalsClient) ([]byte, error) {
	var out strings.Builder
	for {
		frame, err := stream.Recv()
		if err == io.EOF {
			return []byte(out.String()), nil
		}
		if err != nil {
			if out.Len() > 0 && status.Code(err) == codes.DeadlineExceeded {
				return []byte(out.String()), nil
			}
			return []byte(out.String()), err
		}
		data, err := marshalProtoJSONLine(frame)
		if err != nil {
			return []byte(out.String()), err
		}
		out.Write(data)
		out.WriteByte('\n')
	}
}

func collectSignalFramesWithEvents(ctx context.Context, client controlv1.AgentControlServiceClient, reqCtx *controlv1.RequestContext, stream controlv1.AgentControlService_WatchSignalsClient) ([]byte, error) {
	var out strings.Builder
	for {
		frame, err := stream.Recv()
		if err == io.EOF {
			return []byte(out.String()), nil
		}
		if err != nil {
			if out.Len() > 0 && status.Code(err) == codes.DeadlineExceeded {
				return []byte(out.String()), nil
			}
			return []byte(out.String()), err
		}
		data, err := marshalSignalEventEnvelope(ctx, client, reqCtx, frame)
		if err != nil {
			return []byte(out.String()), err
		}
		out.Write(data)
		out.WriteByte('\n')
	}
}

func marshalSignalEventEnvelope(ctx context.Context, client controlv1.AgentControlServiceClient, reqCtx *controlv1.RequestContext, frame *controlv1.SignalFrame) ([]byte, error) {
	signalJSON, err := marshalProtoJSONLine(frame)
	if err != nil {
		return nil, err
	}
	var events []json.RawMessage
	var missing []string
	for _, eventID := range uniqueSignalEventRefs(frame) {
		resp, err := client.GetEvent(ctx, &controlv1.GetEventRequest{Context: reqCtx, EventId: eventID})
		if err != nil {
			missing = append(missing, eventID)
			continue
		}
		eventJSON, err := marshalProtoJSONLine(resp.GetFrame())
		if err != nil {
			return nil, err
		}
		events = append(events, json.RawMessage(eventJSON))
	}
	return json.Marshal(map[string]any{
		"signalFrame":      json.RawMessage(signalJSON),
		"eventFrames":      events,
		"missingEventRefs": missing,
	})
}

func uniqueSignalEventRefs(frame *controlv1.SignalFrame) []string {
	seen := map[string]bool{}
	var out []string
	if frame == nil || frame.GetSignal() == nil {
		return nil
	}
	for _, ref := range frame.GetSignal().GetEventRefs() {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out
}

func policyPayload(args []string) (string, error) {
	file := flagValue(args, "--file")
	if strings.TrimSpace(file) != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if len(args) > 2 && args[2] == "collection" {
		return collectionPolicyPayload(args)
	}
	return "", fmt.Errorf("--file is required")
}

func contentPayload(args []string) (string, error) {
	file := flagValue(args, "--file")
	if strings.TrimSpace(file) == "" {
		return "", fmt.Errorf("--file is required")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func contentDiff(args []string) ([]byte, error) {
	files := flagValues(args, "--file")
	if len(files) != 2 {
		return nil, fmt.Errorf("content diff requires exactly two --file values")
	}
	oldEnv, oldValues, err := readContentValues(files[0])
	if err != nil {
		return nil, err
	}
	newEnv, newValues, err := readContentValues(files[1])
	if err != nil {
		return nil, err
	}
	if oldEnv.Kind != newEnv.Kind || oldEnv.Metadata.ID != newEnv.Metadata.ID {
		return nil, fmt.Errorf("content diff requires same kind and metadata.id")
	}
	oldSet := map[string]bool{}
	for _, value := range oldValues {
		oldSet[value] = true
	}
	newSet := map[string]bool{}
	for _, value := range newValues {
		newSet[value] = true
	}
	var ops []map[string]string
	for _, value := range newValues {
		if !oldSet[value] {
			ops = append(ops, map[string]string{"op": "add", "value": value})
		}
	}
	for _, value := range oldValues {
		if !newSet[value] {
			ops = append(ops, map[string]string{"op": "remove", "value": value})
		}
	}
	version := firstNonEmpty(flagValue(args, "--version"), newEnv.Metadata.Version)
	patch := map[string]any{
		"api_version": newEnv.APIVersion,
		"kind":        newEnv.Kind,
		"metadata": map[string]any{
			"id":      newEnv.Metadata.ID,
			"version": version,
		},
		"spec": map[string]any{
			"base_version":   oldEnv.Metadata.Version,
			"value_type":     contentValueType(newEnv),
			"merge_strategy": "patch",
			"ops":            ops,
		},
	}
	return json.MarshalIndent(patch, "", "  ")
}

type contentEnvelopeForCLI struct {
	APIVersion string `json:"api_version"`
	Kind       string `json:"kind"`
	Metadata   struct {
		ID      string `json:"id"`
		Version string `json:"version"`
	} `json:"metadata"`
	Spec json.RawMessage `json:"spec"`
}

func readContentValues(path string) (contentEnvelopeForCLI, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return contentEnvelopeForCLI{}, nil, err
	}
	var env contentEnvelopeForCLI
	if err := json.Unmarshal(data, &env); err != nil {
		return contentEnvelopeForCLI{}, nil, err
	}
	var spec struct {
		Values []string `json:"values"`
	}
	if err := json.Unmarshal(env.Spec, &spec); err != nil {
		return contentEnvelopeForCLI{}, nil, err
	}
	return env, uniqueSorted(spec.Values), nil
}

func contentValueType(env contentEnvelopeForCLI) string {
	var spec struct {
		ValueType string `json:"value_type"`
	}
	_ = json.Unmarshal(env.Spec, &spec)
	return spec.ValueType
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func collectionPolicyPayload(args []string) (string, error) {
	behaviors := flagValues(args, "--behavior")
	if len(behaviors) == 0 {
		return "", fmt.Errorf("collection policy requires --behavior when --file is not used")
	}
	binaryPrefixes := flagValues(args, "--binary-prefix")
	filePrefixes := flagValues(args, "--file-prefix")
	socketFamilies := flagValues(args, "--socket-family")
	socketAddrs := flagValues(args, "--socket-addr")
	socketPorts := flagValues(args, "--socket-port")
	policy := map[string]any{
		"policy_id":      firstNonEmpty(flagValue(args, "--policy-id"), "local-collection-policy"),
		"version":        uint64Flag(args, "--policy-version", 1),
		"behaviors":      collectionBehaviorPayloads(behaviors, binaryPrefixes, filePrefixes, socketFamilies, socketAddrs, socketPorts),
		"scope_type":     flagValue(args, "--scope-type"),
		"scope_selector": flagValue(args, "--scope-selector"),
		"observe_only":   !hasFlag(args, "--enforce"),
	}
	data, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func collectionBehaviorPayloads(behaviors, binaryPrefixes, filePrefixes, socketFamilies, socketAddrs, socketPorts []string) []map[string]any {
	out := make([]map[string]any, 0, len(behaviors))
	for _, behavior := range behaviors {
		item := map[string]any{"id": behavior, "enabled": true}
		selectors := map[string]any{}
		switch behavior {
		case "process.exec", "process.fork":
			if len(binaryPrefixes) > 0 {
				selectors["binary"] = map[string]any{"prefixes": binaryPrefixes}
			}
		case "file.open", "file.read", "file.write", "file.chmod":
			if len(filePrefixes) > 0 {
				selectors["file"] = map[string]any{"prefixes": filePrefixes}
			}
		case "network.connect":
			socket := map[string]any{}
			if len(socketFamilies) > 0 {
				socket["families"] = socketFamilies
			}
			if len(socketAddrs) > 0 {
				socket["addrs"] = socketAddrs
			}
			if len(socketPorts) > 0 {
				socket["ports"] = socketPorts
			}
			if len(socket) > 0 {
				selectors["socket"] = socket
			}
		}
		if len(selectors) > 0 {
			item["selectors"] = selectors
		}
		out = append(out, item)
	}
	return out
}

func requestContext(args []string) *controlv1.RequestContext {
	return &controlv1.RequestContext{
		RequestId: flagValue(args, "--request-id"),
		TenantId:  flagValue(args, "--tenant-id"),
		AgentId:   flagValue(args, "--agent-id"),
		Scope: &controlv1.Scope{
			Type:     flagValue(args, "--scope-type"),
			Selector: flagValue(args, "--scope-selector"),
		},
	}
}

func watchFilter(args []string) *controlv1.WatchFilter {
	filter := &controlv1.WatchFilter{
		AfterSequence:   uint64Flag(args, "--after-seq", 0),
		SinceObservedAt: flagValue(args, "--since"),
		UntilObservedAt: flagValue(args, "--until"),
		AfterBatchId:    flagValue(args, "--after-batch-id"),
		Labels:          map[string]string{},
	}
	for _, item := range flagValues(args, "--label") {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		filter.Labels[key] = value
	}
	if filter.AfterSequence == 0 && filter.SinceObservedAt == "" && filter.UntilObservedAt == "" && filter.AfterBatchId == "" && len(filter.Labels) == 0 {
		return nil
	}
	return filter
}

func commandTimeout(args []string, fallback time.Duration) time.Duration {
	raw := flagValue(args, "--timeout")
	if raw == "" {
		if len(args) >= 2 && args[1] == "watch" && flagValue(args, "--limit") == "" {
			return 24 * time.Hour
		}
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func uint32Flag(args []string, name string) uint32 {
	raw := flagValue(args, name)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}

func uint64Flag(args []string, name string, fallback uint64) uint64 {
	raw := flagValue(args, name)
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func flagValue(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func flagValues(args []string, name string) []string {
	var out []string
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			out = append(out, args[i+1])
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}

func marshalProtoJSON(msg proto.Message) ([]byte, error) {
	return protojson.MarshalOptions{}.Marshal(msg)
}

func marshalProtoJSONLine(msg proto.Message) ([]byte, error) {
	return protojson.MarshalOptions{}.Marshal(msg)
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
	case "metrics":
		return httpGet(base + "/api/v1/metrics")
	case "store-status":
		return httpGet(base + "/api/v1/store-status")
	case "rarity-baseline":
		q := url.Values{}
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--workload":
				i++
				if i < len(args) {
					q.Set("workload", args[i])
				}
			case "--signal":
				i++
				if i < len(args) {
					q.Set("signal", args[i])
				}
			}
		}
		return httpGet(base + "/api/v1/rarity-baseline?" + q.Encode())
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
	case "operator-role-bindings":
		q := url.Values{}
		req := map[string]any{}
		upsert := false
		for i := 1; i < len(args); i++ {
			switch args[i] {
			case "--upsert":
				upsert = true
			case "--actor":
				i++
				if i < len(args) {
					q.Set("actor", args[i])
					req["actor"] = args[i]
				}
			case "--roles":
				i++
				if i < len(args) {
					req["roles"] = splitCSV(args[i])
				}
			}
		}
		if upsert {
			return httpPostJSON(base+"/api/v1/operator-role-bindings", req)
		}
		return httpGet(base + "/api/v1/operator-role-bindings?" + q.Encode())
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
			case "--behavior":
				i++
				if i < len(args) {
					q.Set("behavior", args[i])
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
	if role := os.Getenv("SYSARMOR_ROLE"); role != "" {
		req.Header.Set("X-SysArmor-Role", role)
	}
}
