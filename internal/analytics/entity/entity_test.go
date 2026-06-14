package entity

import (
	"testing"

	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
)

func TestNormalizePrefixesKnownEntityKeys(t *testing.T) {
	got := Normalize(&signalv1.EntityRef{Kind: " File ", Key: "/dev/shm/x.sh", Role: " Object "})
	if got.GetKind() != "file" || got.GetKey() != "file:/dev/shm/x.sh" || got.GetRole() != "object" {
		t.Fatalf("normalized entity = %#v", got)
	}
}

func TestUniqueDropsDuplicatesAfterNormalization(t *testing.T) {
	got := Unique([]*signalv1.EntityRef{
		{Kind: "socket", Key: "10.66.0.99:443", Role: "object"},
		{Kind: "socket", Key: "socket:10.66.0.99:443", Role: "object"},
		{Kind: "process", Key: "p-bash", Role: "subject"},
	})
	if len(got) != 2 {
		t.Fatalf("unique entities = %d, want 2: %#v", len(got), got)
	}
	if got[0].GetKey() != "socket:10.66.0.99:443" {
		t.Fatalf("socket key = %q", got[0].GetKey())
	}
}
