package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
	policyModel "github.com/sysarmor/sysarmor-next-project/internal/policy"
)

func TestLoadEffectiveEndpointPolicyBootstrapsAndRestoresSQLite(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	store, err := localstore.Open(t.Context(), localstore.Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	path := filepath.Join(t.TempDir(), "policy.json")
	writeEndpointPolicy(t, path, `{"policy_id":"bootstrap","version":1,"collection":{"behaviors":["process.exec"]},"detection":{},"telemetry":{"max_batch_items":64},"response":{}}`)

	first, err := LoadEffectiveEndpointPolicy(t.Context(), store, path)
	if err != nil {
		t.Fatal(err)
	}
	writeEndpointPolicy(t, path, `{"policy_id":"changed-file","version":2,"collection":{"behaviors":["file.read"]},"detection":{},"telemetry":{},"response":{}}`)
	second, err := LoadEffectiveEndpointPolicy(t.Context(), store, path)
	if err != nil {
		t.Fatal(err)
	}
	if first.PolicyID != "bootstrap" || second.PolicyID != "bootstrap" || second.Version != 1 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestEffectiveEndpointPolicyPreservesStructuredCollectionBehaviors(t *testing.T) {
	store, err := localstore.Open(t.Context(), localstore.Options{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	policy, err := ParseEndpointPolicy([]byte(`{
		"policy_id":"structured","version":1,
		"collection":{"behaviors":[{"id":"process.exec","selectors":{"process":{"binary_prefixes":["/bin/"]}}}]},
		"detection":{},"telemetry":{},"response":{}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveEffectiveEndpointPolicy(t.Context(), store, policy); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadEffectiveEndpointPolicy(t.Context(), store, filepath.Join(t.TempDir(), "unused.json"))
	if err != nil {
		t.Fatal(err)
	}
	intent, err := CollectionPolicyIntent(restored.Collection)
	if err != nil {
		t.Fatal(err)
	}
	if len(intent.Behaviors) != 1 || intent.Behaviors[0] != "process.exec" || len(intent.BehaviorFilters[0].BinaryPrefixes) != 1 {
		t.Fatalf("restored collection intent = %+v", intent)
	}
}

func TestCollectionPolicyMarshalOmitsAbsentBehaviors(t *testing.T) {
	raw, err := json.Marshal(policyModel.CollectionPolicy{PolicyID: "empty"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"behaviors"`) {
		t.Fatalf("empty collection policy encoded behaviors: %s", raw)
	}
}

func TestParseEndpointPolicyRequiresAllSections(t *testing.T) {
	if _, err := ParseEndpointPolicy([]byte(`{"policy_id":"bad","version":1,"detection":{},"telemetry":{},"response":{}}`)); err == nil {
		t.Fatal("policy without collection accepted")
	}
}

func TestRepositoryDefaultEndpointPolicyParses(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "deployments", "agent", "policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := ParseEndpointPolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	if policy.PolicyID != "standalone-default" || policy.Version != 1 {
		t.Fatalf("policy=%+v", policy)
	}
	if len(policy.Collection.Behaviors) == 0 || policy.Telemetry.MaxBatchItems != 256 {
		t.Fatalf("policy sections were not normalized: %+v", policy)
	}
}

func writeEndpointPolicy(t *testing.T, path, document string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
}
