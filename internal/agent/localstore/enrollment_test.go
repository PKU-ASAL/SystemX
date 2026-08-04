package localstore

import (
	"testing"
	"time"
)

func TestSetEnrollingValidatesManagedFields(t *testing.T) {
	store := openStore(t, t.TempDir())
	defer store.Close()

	if got, err := store.Enrollment(t.Context()); err != nil || got.State != StateStandalone {
		t.Fatalf("initial enrollment=%+v err=%v", got, err)
	}
	invalid := Enrollment{State: StateEnrolling, TenantID: "default"}
	if err := store.SetEnrolling(t.Context(), invalid); err == nil {
		t.Fatal("incomplete managed enrollment accepted")
	}
	want := Enrollment{
		State: StateEnrolling, TenantID: "default", AgentID: "agent-a", GatewayAddress: "gateway:9444",
		TLSCAPath: "/pki/ca.pem", TLSCertPath: "/pki/agent.pem", TLSKeyPath: "/pki/agent-key.pem",
		ManagedFromSequence: 42,
	}
	if err := store.SetEnrolling(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Enrollment(t.Context()); err != nil || got.AgentID != want.AgentID || got.ManagedFromSequence != 42 {
		t.Fatalf("enrolling enrollment=%+v err=%v", got, err)
	}
}

func TestEnrollmentCanRemainPendingPolicyAuthority(t *testing.T) {
	store := openStore(t, t.TempDir())
	defer store.Close()
	want := Enrollment{TenantID: "tenant-a", AgentID: "agent-a", GatewayAddress: "gateway:9444", TLSCAPath: "/ca", TLSCertPath: "/cert", TLSKeyPath: "/key"}
	if err := store.SetEnrolling(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Enrollment(t.Context())
	if err != nil || got.State != StateEnrolling || got.AgentID != want.AgentID {
		t.Fatalf("enrollment=%+v err=%v", got, err)
	}
}

func TestSetEnrollingRejectsManagedEnrollment(t *testing.T) {
	store := openStore(t, t.TempDir())
	defer store.Close()
	enrollment := Enrollment{TenantID: "tenant-a", AgentID: "agent-a", GatewayAddress: "gateway", TLSCAPath: "/ca", TLSCertPath: "/cert", TLSKeyPath: "/key"}
	activateManagedPolicyForTest(t, store, policyRecord("endpoint", 1, `{"policy_id":"managed"}`))
	if err := store.SetEnrolling(t.Context(), enrollment); err == nil {
		t.Fatal("managed enrollment was demoted to enrolling")
	}
}

func TestUnenrollmentRequiresDurableRevocationConfirmation(t *testing.T) {
	store := openStore(t, t.TempDir())
	defer store.Close()
	standalone := policyRecord("endpoint", 1, `{"policy_id":"standalone"}`)
	if err := store.PutAndActivateStandalonePolicy(t.Context(), standalone); err != nil {
		t.Fatal(err)
	}
	enrollment := Enrollment{
		TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", CertificateSerial: "42",
		GatewayAddress: "gateway", TLSCAPath: "/ca", TLSCertPath: "/cert", TLSKeyPath: "/key",
	}
	if err := store.SetEnrolling(t.Context(), enrollment); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateManagedPolicy(t.Context(), policyRecord("endpoint", 2, `{"policy_id":"managed"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginUnenrollment(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteUnenrollment(t.Context(), "endpoint"); err == nil {
		t.Fatal("unenrollment completed without manager revocation confirmation")
	}
	revokedAt := time.Unix(100, 0).UTC()
	if err := store.ConfirmEnrollmentRevocation(t.Context(), "receipt-a", revokedAt); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteUnenrollment(t.Context(), "endpoint"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Enrollment(t.Context())
	if err != nil || got.State != StateStandalone || got.EnrollmentID != "" || got.CertificateSerial != "" {
		t.Fatalf("completed enrollment=%+v err=%v", got, err)
	}
	_, source, ok, err := store.ActivePolicy(t.Context(), "endpoint")
	if err != nil || !ok || source != PolicySourceStandalone {
		t.Fatalf("active source=%q ok=%t err=%v", source, ok, err)
	}
}
