package daemon

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/localstore"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/sensors/runtime"
)

type enrollmentResult struct {
	Status   string
	Message  string
	Identity runtimeIdentity
}

type enrollmentCoordinator struct {
	runner               *AgentRuntime
	runtime              sensorruntime.Runtime
	lifecycleCtx         context.Context
	completeUnenrollment func(context.Context, string) error
	mu                   sync.Mutex
}

func newEnrollmentCoordinator(lifecycleCtx context.Context, runner *AgentRuntime, runtime sensorruntime.Runtime) *enrollmentCoordinator {
	return &enrollmentCoordinator{runner: runner, runtime: runtime, lifecycleCtx: lifecycleCtx, completeUnenrollment: runner.localStore.CompleteUnenrollment}
}

func (c *enrollmentCoordinator) Enroll(ctx context.Context, managerURL, token string, uploadHistory bool) enrollmentResult {
	if c.runner.localStore == nil {
		return c.result("rejected", "local store is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if result, handled := c.existingEnrollment(ctx); handled {
		return result
	}
	managerURL, err := normalizeManagerURL(managerURL)
	if err != nil {
		return c.result("rejected", err.Error())
	}
	certificate, keyPEM, pendingKeyPath, err := requestEnrollmentCertificate(ctx, managerURL, token, c.runner.Config.Local.StatePath)
	if err != nil {
		return c.result("rejected", err.Error())
	}
	paths, created, err := writeEnrollmentCredentials(c.runner.Config.Local.StatePath, certificate, keyPEM)
	if err != nil {
		return c.result("rejected", fmt.Sprintf("write credentials: %v", err))
	}
	stats, err := c.runner.localStore.Stats(ctx)
	if err != nil {
		return c.result("rejected", rollbackEnrollmentFailure(paths, created, err).Error())
	}
	fromSequence := stats.LatestEventSequence + 1
	if uploadHistory {
		fromSequence = stats.OldestEventSequence
	}
	enrollment := localstore.Enrollment{State: localstore.StateEnrolling, TenantID: certificate.TenantID, AgentID: certificate.AgentID, EnrollmentID: certificate.EnrollmentID, CertificateSerial: certificate.SerialNumber, ManagerURL: managerURL, GatewayAddress: certificate.GatewayAddress, TLSCAPath: paths.CA, TLSCertPath: paths.Certificate, TLSKeyPath: paths.Key, TLSServerName: certificate.GatewayServerName, UploadHistory: uploadHistory, ManagedFromSequence: fromSequence}
	if c.runner.network != nil {
		c.runner.network.Stop()
	}
	c.runner.policyAuthorityMu.Lock()
	err = c.runner.localStore.SetEnrolling(ctx, enrollment)
	if err == nil {
		c.runner.applyEnrollmentIdentity(enrollment)
	}
	c.runner.policyAuthorityMu.Unlock()
	if err != nil {
		if c.runner.network != nil {
			c.runner.network.ApplyEnrollment(localstore.Enrollment{State: localstore.StateStandalone})
		}
		return c.result("rejected", rollbackEnrollmentFailure(paths, created, err).Error())
	}
	if c.runner.network != nil {
		c.runner.network.ApplyEnrollment(enrollment)
	}
	if err := os.Remove(pendingKeyPath); err != nil && !os.IsNotExist(err) && c.runner.Out != nil {
		fmt.Fprintf(c.runner.Out, "remove pending enrollment key: %v\n", err)
	}
	return c.result("pending", "enrollment credentials accepted; waiting for manager endpoint policy")
}

func (c *enrollmentCoordinator) Unenroll(ctx context.Context) enrollmentResult {
	if c.runner.localStore == nil {
		return c.result("rejected", "local store is unavailable")
	}
	c.mu.Lock()
	result, locallyComplete := c.unenrollLocked(ctx)
	c.mu.Unlock()
	if !locallyComplete {
		return result
	}
	if _, err := c.reportCompletion(ctx); err != nil {
		return c.result("pending", "agent returned to standalone mode; manager completion is pending: "+err.Error())
	}
	return c.result("applied", "manager confirmed endpoint unenrollment completion")
}

func (c *enrollmentCoordinator) unenrollLocked(ctx context.Context) (enrollmentResult, bool) {
	current, completion, err := c.prepareUnenrollment(ctx)
	if err != nil {
		return c.result("rejected", err.Error()), false
	}
	if !current.RevocationConfirmed {
		revoke := c.runner.revokeEnrollment
		if revoke == nil {
			revoke = revokeEnrollmentOnline
		}
		receipt, revokedAt, err := revoke(ctx, current, completion.TokenHash)
		if err != nil {
			_ = c.runner.localStore.RecordUnenrollmentError(ctx, err.Error())
			return c.result("pending", "manager certificate revocation is pending: "+err.Error()), false
		}
		if err := c.runner.localStore.ConfirmEnrollmentRevocation(ctx, receipt, revokedAt); err != nil {
			return c.result("rejected", err.Error()), false
		}
	}
	if err := c.completeConfirmed(c.lifecycleCtx, current); err != nil {
		return c.result("rejected", err.Error()), false
	}
	if completion.TokenHash == "" {
		return c.result("applied", "manager revoked legacy enrollment; agent returned to standalone mode"), false
	}
	return enrollmentResult{}, true
}

func (c *enrollmentCoordinator) Resume(ctx context.Context) error {
	if c.runner.localStore == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	current, err := c.runner.localStore.Enrollment(ctx)
	if err != nil {
		return fmt.Errorf("read enrollment for recovery: %w", err)
	}
	if current.State != localstore.StateUnenrolling || !current.RevocationConfirmed {
		return nil
	}
	return c.completeConfirmed(ctx, current)
}

func (c *enrollmentCoordinator) existingEnrollment(ctx context.Context) (enrollmentResult, bool) {
	enrollment, err := c.runner.localStore.Enrollment(ctx)
	if err != nil {
		return c.result("rejected", "read enrollment state: "+err.Error()), true
	}
	switch enrollment.State {
	case localstore.StateStandalone:
		if _, pending, err := c.runner.localStore.UnenrollmentCompletion(ctx); err != nil {
			return c.result("rejected", "read unenrollment completion: "+err.Error()), true
		} else if pending {
			return c.result("pending", "manager unenrollment completion is pending acknowledgement"), true
		}
		return enrollmentResult{}, false
	case localstore.StateEnrolling:
		return c.result("pending", "enrollment is already waiting for manager endpoint policy"), true
	case localstore.StateManaged:
		return c.result("rejected", "agent is already managed"), true
	case localstore.StateUnenrolling:
		return c.result("rejected", "agent unenrollment is pending manager confirmation"), true
	default:
		return c.result("rejected", "unsupported enrollment state"), true
	}
}

func (c *enrollmentCoordinator) completeConfirmed(ctx context.Context, current localstore.Enrollment) error {
	if c.runner.network != nil {
		c.runner.network.Stop()
	}
	// Wait for in-flight authority mutations, but never hold this lock while
	// sensor reconciliation invokes callbacks that acquire the same lock.
	c.runner.policyAuthorityMu.Lock()
	c.runner.policyAuthorityMu.Unlock()
	err := restoreStandaloneEndpointPolicyWithActivation(ctx, c.runner, c.runtime, func(ctx context.Context) error {
		if failures := removeCredentials(credentialPaths{CA: current.TLSCAPath, Certificate: current.TLSCertPath, Key: current.TLSKeyPath}); len(failures) > 0 {
			return fmt.Errorf("remove enrollment credentials: %s", strings.Join(failures, "; "))
		}
		return c.completeUnenrollment(ctx, "endpoint")
	})
	if err != nil {
		return err
	}
	standalone := localstore.Enrollment{State: localstore.StateStandalone}
	if c.runner.network != nil {
		c.runner.network.ApplyEnrollment(standalone)
	}
	c.runner.applyEnrollmentIdentity(standalone)
	return nil
}

func (c *enrollmentCoordinator) prepareUnenrollment(ctx context.Context) (localstore.Enrollment, localstore.UnenrollmentCompletion, error) {
	current, err := c.runner.localStore.Enrollment(ctx)
	if err != nil {
		return localstore.Enrollment{}, localstore.UnenrollmentCompletion{}, err
	}
	if current.UnenrollmentProtocol == localstore.UnenrollmentProtocolLegacyMTLS {
		current, err = c.runner.localStore.BeginUnenrollment(ctx)
		return current, localstore.UnenrollmentCompletion{}, err
	}
	if current.UnenrollmentProtocol != localstore.UnenrollmentProtocolCompletionV1 {
		return localstore.Enrollment{}, localstore.UnenrollmentCompletion{}, fmt.Errorf("unsupported unenrollment protocol %q", current.UnenrollmentProtocol)
	}
	if current.State == localstore.StateUnenrolling {
		completion, ok, err := c.runner.localStore.UnenrollmentCompletion(ctx)
		if err != nil {
			return localstore.Enrollment{}, localstore.UnenrollmentCompletion{}, fmt.Errorf("read prepared unenrollment completion: %w", err)
		}
		if !ok {
			return localstore.Enrollment{}, localstore.UnenrollmentCompletion{}, fmt.Errorf("prepared unenrollment completion does not exist")
		}
		return current, completion, nil
	}
	token, tokenHash, err := newUnenrollmentCompletionToken()
	if err != nil {
		return localstore.Enrollment{}, localstore.UnenrollmentCompletion{}, err
	}
	current, err = c.runner.localStore.PrepareUnenrollment(ctx, token, tokenHash)
	if err != nil {
		return localstore.Enrollment{}, localstore.UnenrollmentCompletion{}, err
	}
	completion, ok, err := c.runner.localStore.UnenrollmentCompletion(ctx)
	if err != nil {
		return localstore.Enrollment{}, localstore.UnenrollmentCompletion{}, fmt.Errorf("read prepared unenrollment completion: %w", err)
	}
	if !ok {
		return localstore.Enrollment{}, localstore.UnenrollmentCompletion{}, fmt.Errorf("prepared unenrollment completion does not exist")
	}
	return current, completion, nil
}

func newUnenrollmentCompletionToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generate unenrollment completion token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:]), nil
}

func (c *enrollmentCoordinator) reportCompletion(ctx context.Context) (bool, error) {
	if c.runner.reportUnenrollment != nil {
		return c.runner.reportUnenrollment(ctx)
	}
	if c.runner.completionReporter == nil {
		return false, fmt.Errorf("unenrollment completion reporter is unavailable")
	}
	return c.runner.completionReporter.ReportOnce(ctx)
}

func (c *enrollmentCoordinator) result(status, message string) enrollmentResult {
	return enrollmentResult{Status: status, Message: message, Identity: c.runner.currentIdentity()}
}
