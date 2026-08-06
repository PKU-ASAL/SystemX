package daemon

import (
	"encoding/json"
	"testing"

	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/config"
	controlmodel "github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/incident/v1"
	responsemodel "github.com/sysarmor/sysarmor-next-project/packages/response"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestResponseControllerObserveModeBindsRuntimeIdentity(t *testing.T) {
	runner := &AgentRuntime{Config: config.Config{Agent: config.AgentConfig{TenantID: "tenant-a", ID: "agent-a"}}}
	ack := newResponseController(runner).ExecuteResponse(t.Context(), responsemodel.Command{
		ResponseID: "response-a", Mode: "observe", Action: "kill", Target: "process:42",
	})
	if !ack.Accepted || !ack.ObserveOnly || ack.Executed || ack.TenantID != "tenant-a" || ack.AgentID != "agent-a" {
		t.Fatalf("ack=%+v", ack)
	}
}

func TestResponseControllerCollectEvidenceBuildsSubgraph(t *testing.T) {
	runner := &AgentRuntime{Config: config.Config{Agent: config.AgentConfig{TenantID: "tenant-a", ID: "agent-a"}}}
	result := newResponseController(runner).CollectEvidence(t.Context(), controlmodel.EvidencePullbackRequest{
		RequestID: "request-a", Target: "process:42",
	})
	if !result.OK || result.TenantID != "tenant-a" || result.AgentID != "agent-a" || !json.Valid(result.Evidence) {
		t.Fatalf("result=%+v", result)
	}
	var evidence incidentv1.EvidenceSubgraph
	if err := protojson.Unmarshal(result.Evidence, &evidence); err != nil {
		t.Fatal(err)
	}
	if len(evidence.GetNodes()) != 1 || evidence.GetNodes()[0].GetKind() != "process" {
		t.Fatalf("evidence=%+v", evidence.GetNodes())
	}
}
