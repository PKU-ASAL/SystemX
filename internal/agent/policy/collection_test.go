package policy

import (
	"path/filepath"
	"testing"
)

func TestParseCollectionIntent(t *testing.T) {
	intent, err := ParseCollectionIntent(`{"behaviors":["process.exec","process.exit","process.fork","file.open","file.write","file.chmod","network.connect"]}`, true)
	if err != nil {
		t.Fatalf("ParseCollectionIntent() error = %v", err)
	}
	if !intent.ObserveOnly {
		t.Fatal("ObserveOnly = false")
	}
	if len(intent.Behaviors) != 7 {
		t.Fatalf("Behaviors len = %d", len(intent.Behaviors))
	}
	if intent.Behaviors[0] != "process.exec" {
		t.Fatalf("first behavior = %v", intent.Behaviors[0])
	}
}

func TestLoadCollectionIntentFromRepoPolicy(t *testing.T) {
	path := filepath.Join("..", "..", "..", "test", "policies", "collection.yaml")
	intent, err := LoadCollectionIntent(path, true)
	if err != nil {
		t.Fatalf("LoadCollectionIntent() error = %v", err)
	}
	if len(intent.Behaviors) == 0 {
		t.Fatal("Behaviors is empty")
	}
}

func TestParseCollectionIntentRequiresJSONBehaviors(t *testing.T) {
	_, err := ParseCollectionIntent(`kinds: [SETUID]`, true)
	if err == nil {
		t.Fatal("ParseCollectionIntent() error = nil")
	}
}

func TestParseCollectionPolicyJSONBehaviorsAndFilters(t *testing.T) {
	policy, err := ParseCollectionPolicyJSON([]byte(`{
		"policy_id":"collection-a",
		"version":2,
		"behaviors":["network.connect","file.write"],
		"binary_prefixes":["/var/lib/app/plugins"],
		"file_prefixes":["/dev/shm","/var/lib/app/plugins"],
		"socket_families":["AF_INET"],
		"socket_addrs":["10.66.0.99"],
		"socket_ports":["443","8080"],
		"scope_type":"container",
		"scope_selector":"abc123",
		"observe_only":true
	}`), true)
	if err != nil {
		t.Fatalf("ParseCollectionPolicyJSON() error = %v", err)
	}
	intent, err := CollectionPolicyIntent(policy)
	if err != nil {
		t.Fatalf("CollectionPolicyIntent() error = %v", err)
	}
	if len(intent.Behaviors) != 2 {
		t.Fatalf("Behaviors = %v", intent.Behaviors)
	}
	if intent.Behaviors[0] != "network.connect" {
		t.Fatalf("first behavior = %v", intent.Behaviors[0])
	}
	if got := intent.FilePrefixes; len(got) != 2 || got[0] != "/dev/shm" {
		t.Fatalf("FilePrefixes = %v", got)
	}
	if got := intent.BinaryPrefixes; len(got) != 1 || got[0] != "/var/lib/app/plugins" {
		t.Fatalf("BinaryPrefixes = %v", got)
	}
	if got := intent.SocketFamilies; len(got) != 1 || got[0] != "AF_INET" {
		t.Fatalf("SocketFamilies = %v", got)
	}
	if got := intent.SocketAddrs; len(got) != 1 || got[0] != "10.66.0.99" {
		t.Fatalf("SocketAddrs = %v", got)
	}
	if got := intent.SocketPorts; len(got) != 2 || got[0] != "443" {
		t.Fatalf("SocketPorts = %v", got)
	}
	if intent.ScopeType != "container" || intent.ScopeSelector != "abc123" {
		t.Fatalf("scope = %s/%s", intent.ScopeType, intent.ScopeSelector)
	}
}

func TestCollectionPolicyRejectsUnknownBehavior(t *testing.T) {
	_, err := CollectionPolicyIntent(CollectionPolicy{Behaviors: []string{"unknown.behavior"}})
	if err == nil {
		t.Fatal("CollectionPolicyIntent() error = nil")
	}
}
