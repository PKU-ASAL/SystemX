package policy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sysarmor/sysarmor-next-project/internal/agent/localstore"
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
