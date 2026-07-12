package localstore

import "testing"

func TestEnrollmentTransitionsValidateManagedFields(t *testing.T) {
	store := openStore(t, t.TempDir())
	defer store.Close()

	if got, err := store.Enrollment(t.Context()); err != nil || got.State != StateStandalone {
		t.Fatalf("initial enrollment=%+v err=%v", got, err)
	}
	invalid := Enrollment{State: StateManaged, TenantID: "default"}
	if err := store.SetManaged(t.Context(), invalid); err == nil {
		t.Fatal("incomplete managed enrollment accepted")
	}
	want := Enrollment{
		State: StateManaged, TenantID: "default", AgentID: "agent-a", GatewayAddress: "gateway:9444",
		TLSCAPath: "/pki/ca.pem", TLSCertPath: "/pki/agent.pem", TLSKeyPath: "/pki/agent-key.pem",
		ManagedFromSequence: 42,
	}
	if err := store.SetManaged(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Enrollment(t.Context()); err != nil || got.AgentID != want.AgentID || got.ManagedFromSequence != 42 {
		t.Fatalf("managed enrollment=%+v err=%v", got, err)
	}
	if err := store.SetStandalone(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Enrollment(t.Context()); err != nil || got.State != StateStandalone || got.AgentID != "" {
		t.Fatalf("standalone enrollment=%+v err=%v", got, err)
	}
}
