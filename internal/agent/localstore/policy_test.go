package localstore

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestPolicyReplaceSurvivesReopen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agent")
	store := openStore(t, root)
	want := PolicyRecord{Kind: "detection", Version: 7, Document: []byte(`{"policy_id":"p1"}`), Digest: "sha256:first"}
	if err := store.PutPolicy(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	want.Version = 8
	want.Document = []byte(`{"policy_id":"p2"}`)
	want.Digest = "sha256:second"
	if err := store.PutPolicy(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, root)
	defer store.Close()
	got, ok, err := store.Policy(t.Context(), "detection")
	if err != nil || !ok {
		t.Fatalf("policy ok=%t err=%v", ok, err)
	}
	if got.Version != want.Version || got.Digest != want.Digest || !bytes.Equal(got.Document, want.Document) {
		t.Fatalf("policy=%+v want=%+v", got, want)
	}
}
