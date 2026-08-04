package localstore

import (
	"strings"
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

func TestPrepareUnenrollmentPersistsCompletionBeforeRevocation(t *testing.T) {
	store := managedEnrollmentStore(t)
	tokenHash := strings.Repeat("a", 64)

	current, err := store.PrepareUnenrollment(t.Context(), "completion-token", tokenHash)
	if err != nil {
		t.Fatal(err)
	}
	completion, ok, err := store.UnenrollmentCompletion(t.Context())
	if err != nil || !ok {
		t.Fatalf("completion=%+v ok=%t err=%v", completion, ok, err)
	}
	if current.State != StateUnenrolling || current.TransitionPhase != "revocation_pending" || completion.Status != CompletionPrepared ||
		completion.EnrollmentID != "enroll-a" || completion.ManagerURL != "https://manager.example" || completion.TokenHash != tokenHash {
		t.Fatalf("enrollment=%+v completion=%+v", current, completion)
	}
}

func TestCompleteUnenrollmentAtomicallyMarksCompletionReady(t *testing.T) {
	store := managedEnrollmentStore(t)
	if _, err := store.PrepareUnenrollment(t.Context(), "completion-token", strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmEnrollmentRevocation(t.Context(), "receipt-a", time.Unix(100, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteUnenrollment(t.Context(), "endpoint"); err != nil {
		t.Fatal(err)
	}

	enrollment, err := store.Enrollment(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	completion, ok, err := store.UnenrollmentCompletion(t.Context())
	if err != nil || !ok {
		t.Fatalf("completion=%+v ok=%t err=%v", completion, ok, err)
	}
	if enrollment.State != StateStandalone || completion.Status != CompletionReady || completion.RevocationReceipt != "receipt-a" {
		t.Fatalf("enrollment=%+v completion=%+v", enrollment, completion)
	}
}

func TestCompletionAcknowledgementRequiresMatchingEnrollment(t *testing.T) {
	store := managedEnrollmentStore(t)
	if _, err := store.PrepareUnenrollment(t.Context(), "completion-token", strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmEnrollmentRevocation(t.Context(), "receipt-a", time.Unix(100, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteUnenrollment(t.Context(), "endpoint"); err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgeUnenrollmentCompletion(t.Context(), "other-enrollment"); err == nil {
		t.Fatal("mismatched enrollment acknowledged completion")
	}
	if _, ok, err := store.UnenrollmentCompletion(t.Context()); err != nil || !ok {
		t.Fatalf("completion removed after mismatch: ok=%t err=%v", ok, err)
	}
	if err := store.AcknowledgeUnenrollmentCompletion(t.Context(), "enroll-a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.UnenrollmentCompletion(t.Context()); err != nil || ok {
		t.Fatalf("completion remains after acknowledgement: ok=%t err=%v", ok, err)
	}
}

func TestCompleteUnenrollmentKeepsPreparedCompletionOnPolicyFailure(t *testing.T) {
	store := openStore(t, t.TempDir())
	defer store.Close()
	enrollment := Enrollment{
		TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", CertificateSerial: "42",
		ManagerURL: "https://manager.example", GatewayAddress: "gateway", TLSCAPath: "/ca", TLSCertPath: "/cert", TLSKeyPath: "/key",
	}
	if err := store.SetEnrolling(t.Context(), enrollment); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateManagedPolicy(t.Context(), policyRecord("endpoint", 2, `{"policy_id":"managed"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PrepareUnenrollment(t.Context(), "completion-token", strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := store.ConfirmEnrollmentRevocation(t.Context(), "receipt-a", time.Unix(100, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteUnenrollment(t.Context(), "endpoint"); err == nil {
		t.Fatal("completion without standalone policy succeeded")
	}

	current, err := store.Enrollment(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	completion, ok, err := store.UnenrollmentCompletion(t.Context())
	if err != nil || !ok || current.State != StateUnenrolling || completion.Status != CompletionPrepared {
		t.Fatalf("enrollment=%+v completion=%+v ok=%t err=%v", current, completion, ok, err)
	}
}

func managedEnrollmentStore(t *testing.T) *Store {
	t.Helper()
	store := openStore(t, t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	if err := store.PutAndActivateStandalonePolicy(t.Context(), policyRecord("endpoint", 1, `{"policy_id":"standalone"}`)); err != nil {
		t.Fatal(err)
	}
	enrollment := Enrollment{
		TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", CertificateSerial: "42",
		ManagerURL: "https://manager.example", GatewayAddress: "gateway", TLSCAPath: "/ca", TLSCertPath: "/cert", TLSKeyPath: "/key",
	}
	if err := store.SetEnrolling(t.Context(), enrollment); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivateManagedPolicy(t.Context(), policyRecord("endpoint", 2, `{"policy_id":"managed"}`)); err != nil {
		t.Fatal(err)
	}
	return store
}
