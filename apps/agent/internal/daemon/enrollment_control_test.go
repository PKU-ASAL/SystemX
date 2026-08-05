package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/config"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/localstore"
	agentpolicy "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/policy"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/sensors/runtime"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	"github.com/sysarmor/sysarmor-next-project/packages/sensor-sdk/contract"
)

func TestUnenrollDoesNotWaitForManagedFlowWhileHoldingPolicyAuthority(t *testing.T) {
	store := openEndpointPolicyStore(t)
	defer store.Close()
	standalone := parseEndpointPolicy(t, standaloneEndpointPolicyJSON)
	if err := agentpolicy.SaveEffectiveEndpointPolicy(t.Context(), store, standalone); err != nil {
		t.Fatal(err)
	}
	setManagedEnrollmentForTest(t, store)
	runner := newEndpointPolicyRunner(t, store, &healthOnlySensor{health: contract.Health{Backend: "fake"}})
	runner.revokeEnrollment = func(context.Context, localstore.Enrollment, string) (string, time.Time, error) {
		return "receipt-a", time.Now().UTC(), nil
	}
	runner.setEndpointPolicy(standalone)
	flowStarted := make(chan struct{})
	var flowStartedOnce sync.Once
	runner.network = newNetworkSupervisor(t.Context(), func(ctx context.Context) { <-ctx.Done() }, func(ctx context.Context, _ localstore.Enrollment) {
		flowStartedOnce.Do(func() { close(flowStarted) })
		<-ctx.Done()
		runner.policyAuthorityMu.Lock()
		runner.policyAuthorityMu.Unlock()
	})
	runner.network.ApplyEnrollment(localstore.Enrollment{State: localstore.StateManaged, AgentID: "agent-a"})
	<-flowStarted
	server := &localControlServer{runner: runner, runtime: sensorruntime.New(runner.Sensor)}
	done := make(chan *controlplanev1.ControlAck, 1)
	go func() {
		ack, _ := server.Unenroll(t.Context(), &controlplanev1.UnenrollRequest{})
		done <- ack
	}()
	select {
	case ack := <-done:
		if ack.GetStatus() != "applied" {
			t.Fatalf("Unenroll() ack = %+v", ack)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Unenroll blocked while managed flow waited for policy authority")
	}
}

func TestUnenrollRemainsManagedWhenManagerRevocationIsUnconfirmed(t *testing.T) {
	store := openEndpointPolicyStore(t)
	defer store.Close()
	if err := agentpolicy.SaveEffectiveEndpointPolicy(t.Context(), store, parseEndpointPolicy(t, standaloneEndpointPolicyJSON)); err != nil {
		t.Fatal(err)
	}
	setManagedEnrollmentForTest(t, store)
	runner := newEndpointPolicyRunner(t, store, &healthOnlySensor{health: contract.Health{Backend: "fake"}})
	runner.revokeEnrollment = func(context.Context, localstore.Enrollment, string) (string, time.Time, error) {
		return "", time.Time{}, context.DeadlineExceeded
	}
	server := &localControlServer{runner: runner, runtime: sensorruntime.New(runner.Sensor)}

	ack, err := server.Unenroll(t.Context(), &controlplanev1.UnenrollRequest{})
	got, readErr := store.Enrollment(t.Context())
	if err != nil || ack.GetStatus() != "pending" || readErr != nil || got.State != localstore.StateUnenrolling || got.RevocationConfirmed {
		t.Fatalf("ack=%+v err=%v enrollment=%+v readErr=%v", ack, err, got, readErr)
	}
	_, source, ok, activeErr := store.ActivePolicy(t.Context(), "endpoint")
	if activeErr != nil || !ok || source != localstore.PolicySourceManaged {
		t.Fatalf("active source=%q ok=%t err=%v", source, ok, activeErr)
	}
}

func TestEnrollReturnsPendingWithoutRequestingAnotherCertificate(t *testing.T) {
	store, err := localstore.Open(t.Context(), localstore.Options{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	enrollment := testRemoteEnrollment()
	if err := store.SetEnrolling(t.Context(), enrollment); err != nil {
		t.Fatal(err)
	}
	server := &localControlServer{runner: &AgentRuntime{Config: config.Config{Local: config.LocalConfig{StatePath: t.TempDir()}}, localStore: store}}
	ack, err := server.Enroll(t.Context(), &controlplanev1.EnrollRequest{ManagerUrl: "://invalid"})
	if err != nil || ack.GetStatus() != "pending" || !strings.Contains(ack.GetMessage(), "already waiting") {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
}

func TestEnrollRejectsManagedAgentWithoutRequestingAnotherCertificate(t *testing.T) {
	store, err := localstore.Open(t.Context(), localstore.Options{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	setManagedEnrollmentForTest(t, store)
	server := &localControlServer{runner: &AgentRuntime{Config: config.Config{Local: config.LocalConfig{StatePath: t.TempDir()}}, localStore: store}}
	ack, err := server.Enroll(t.Context(), &controlplanev1.EnrollRequest{ManagerUrl: "://invalid"})
	if err != nil || ack.GetStatus() != "rejected" || !strings.Contains(ack.GetMessage(), "already managed") {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
}

func testRemoteEnrollment() localstore.Enrollment {
	return localstore.Enrollment{TenantID: "tenant-a", AgentID: "agent-a", EnrollmentID: "enroll-a", CertificateSerial: "42", ManagerURL: "https://manager.example", GatewayAddress: "gateway", TLSCAPath: "/ca", TLSCertPath: "/cert", TLSKeyPath: "/key"}
}

func setManagedEnrollmentForTest(t *testing.T, store *localstore.Store) {
	t.Helper()
	enrollment := testRemoteEnrollment()
	credentialDir := t.TempDir()
	enrollment.TLSCAPath = filepath.Join(credentialDir, "ca.pem")
	enrollment.TLSCertPath = filepath.Join(credentialDir, "agent.pem")
	enrollment.TLSKeyPath = filepath.Join(credentialDir, "agent-key.pem")
	if err := store.SetEnrolling(t.Context(), enrollment); err != nil {
		t.Fatal(err)
	}
	policy := parseEndpointPolicy(t, standaloneEndpointPolicyJSON)
	policy.PolicyID = "managed"
	if err := agentpolicy.ActivateManagedEndpointPolicy(t.Context(), store, policy); err != nil {
		t.Fatal(err)
	}
}
