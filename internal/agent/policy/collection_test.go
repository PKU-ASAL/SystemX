package policy

import (
	"path/filepath"
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
)

func TestParseCollectionIntent(t *testing.T) {
	intent, err := ParseCollectionIntent(`spec:
  collection:
    kinds: [EXEC, EXIT, FORK, OPEN, WRITE, CHMOD, CONNECT, SETUID]
`, true)
	if err != nil {
		t.Fatalf("ParseCollectionIntent() error = %v", err)
	}
	if !intent.ObserveOnly {
		t.Fatal("ObserveOnly = false")
	}
	if len(intent.EventKinds) != 7 {
		t.Fatalf("EventKinds len = %d", len(intent.EventKinds))
	}
	if intent.EventKinds[0] != eventv1.EventKind_EVENT_KIND_EXEC {
		t.Fatalf("first kind = %v", intent.EventKinds[0])
	}
}

func TestLoadCollectionIntentFromRepoPolicy(t *testing.T) {
	path := filepath.Join("..", "..", "..", "test", "policies", "collection.yaml")
	intent, err := LoadCollectionIntent(path, true)
	if err != nil {
		t.Fatalf("LoadCollectionIntent() error = %v", err)
	}
	if len(intent.EventKinds) == 0 {
		t.Fatal("EventKinds is empty")
	}
}

func TestParseCollectionIntentRequiresSupportedKinds(t *testing.T) {
	_, err := ParseCollectionIntent(`kinds: [SETUID]`, true)
	if err == nil {
		t.Fatal("ParseCollectionIntent() error = nil")
	}
}
