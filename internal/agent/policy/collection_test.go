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
		"behaviors":[
			{
				"id":"network.connect",
				"selectors":{
					"process":{"binary_prefixes":["/var/lib/app/plugins"]},
					"socket":{"families":["AF_INET"],"addrs":["10.66.0.99"],"ports":["443","8080"]}
				}
			},
			{
				"id":"file.write",
				"selectors":{
					"file":{"prefixes":["/dev/shm","/var/lib/app/plugins"]}
				}
			}
		],
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
	if len(intent.BehaviorFilters) != 2 {
		t.Fatalf("BehaviorFilters = %+v", intent.BehaviorFilters)
	}
	network := intent.BehaviorFilters[0]
	if network.Behavior != "network.connect" || len(network.SocketFamilies) != 1 || network.SocketFamilies[0] != "AF_INET" {
		t.Fatalf("network filter = %+v", network)
	}
	file := intent.BehaviorFilters[1]
	if file.Behavior != "file.write" || len(file.FilePrefixes) != 2 || file.FilePrefixes[0] != "/dev/shm" {
		t.Fatalf("file filter = %+v", file)
	}
	if intent.ScopeType != "container" || intent.ScopeSelector != "abc123" {
		t.Fatalf("scope = %s/%s", intent.ScopeType, intent.ScopeSelector)
	}
}

func TestCollectionPolicyFlatFieldsBecomeBehaviorFilters(t *testing.T) {
	policy, err := ParseCollectionPolicyJSON([]byte(`{
		"behaviors":["network.connect","file.write"],
		"file_prefixes":["/dev/shm"],
		"socket_families":["AF_INET"]
	}`), true)
	if err != nil {
		t.Fatalf("ParseCollectionPolicyJSON() error = %v", err)
	}
	intent, err := CollectionPolicyIntent(policy)
	if err != nil {
		t.Fatalf("CollectionPolicyIntent() error = %v", err)
	}
	if len(intent.BehaviorFilters) != 2 {
		t.Fatalf("BehaviorFilters = %+v", intent.BehaviorFilters)
	}
	if got := intent.BehaviorFilters[0].SocketFamilies; len(got) != 1 || got[0] != "AF_INET" {
		t.Fatalf("network socket families = %v", got)
	}
	if got := intent.BehaviorFilters[1].FilePrefixes; len(got) != 1 || got[0] != "/dev/shm" {
		t.Fatalf("file prefixes = %v", got)
	}
}

func TestCollectionPolicyRejectsUnknownBehavior(t *testing.T) {
	_, err := CollectionPolicyIntent(CollectionPolicy{Behaviors: []string{"unknown.behavior"}})
	if err == nil {
		t.Fatal("CollectionPolicyIntent() error = nil")
	}
}
