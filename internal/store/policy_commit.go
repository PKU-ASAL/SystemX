package store

import (
	"encoding/json"
	"fmt"
	"time"

	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
)

func (s *Store) PublishPolicyWithAudit(tenantID, policyID string, version uint64, published bool, audit policymodel.AuditRecord) (policymodel.Policy, bool, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	if err := s.loadPolicyForCommit(tenantID, policyID, version); err != nil {
		return policymodel.Policy{}, false, err
	}
	s.mu.RLock()
	index := policyIndex(s.Policies, tenantID, policyID, version)
	if index < 0 {
		s.mu.RUnlock()
		return policymodel.Policy{}, false, nil
	}
	policy := s.Policies[index]
	backend, ctx := s.backend, s.baseCtx
	s.mu.RUnlock()
	policy.Published = published
	policy.UpdatedAt = time.Now().UTC()
	audit.TenantID, audit.PolicyID, audit.PolicyVersion = policy.TenantID, policy.PolicyID, policy.Version
	audit = normalizePolicyAudit(audit)
	if backend != nil {
		if err := backend.CommitPolicyPublication(ctxOrBackground(ctx), policy, audit); err != nil {
			return policymodel.Policy{}, false, fmt.Errorf("commit policy publication: %w", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	policies, _ := upsertPolicySnapshot(s.Policies, policy)
	audits := upsertPolicyAuditSnapshot(s.PolicyAudits, audit)
	if err := s.applyPolicyStateLocked(policies, nil, audits, nil); err != nil {
		return policymodel.Policy{}, false, fmt.Errorf("commit policy publication: %w", err)
	}
	return policy, true, nil
}

func (s *Store) loadPolicyForCommit(tenantID, policyID string, version uint64) error {
	s.mu.RLock()
	loaded := policyIndex(s.Policies, tenantID, policyID, version) >= 0
	s.mu.RUnlock()
	backend, ctx := s.backendCtx()
	if loaded || backend == nil {
		return nil
	}
	policy, ok, err := backend.GetPolicy(ctx, tenantID, policyID, version)
	if err != nil {
		return fmt.Errorf("load policy for publication: %w", err)
	}
	if !ok {
		return nil
	}
	s.mu.Lock()
	if policyIndex(s.Policies, tenantID, policyID, version) < 0 {
		s.Policies = append(s.Policies, policy)
	}
	s.mu.Unlock()
	return nil
}

func policyIndex(policies []policymodel.Policy, tenantID, policyID string, version uint64) int {
	index := -1
	for i, policy := range policies {
		if policy.TenantID != tenantID || policy.PolicyID != policyID || (version != 0 && policy.Version != version) {
			continue
		}
		if version != 0 {
			return i
		}
		if index < 0 || policy.Version > policies[index].Version {
			index = i
		}
	}
	return index
}

func (s *Store) AssignPolicyWithAudit(assignment policymodel.Assignment, audit policymodel.AuditRecord, command *controlmodel.ControlCommand) (policymodel.Assignment, *controlmodel.ControlCommand, bool, error) {
	if assignment.TenantID == "" {
		assignment.TenantID = "default"
	}
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	if err := s.loadAssignmentState(assignment); err != nil {
		return policymodel.Assignment{}, nil, false, err
	}
	s.mu.RLock()
	policies := append([]policymodel.Policy(nil), s.Policies...)
	assignments := append([]policymodel.Assignment(nil), s.Assignments...)
	backend, ctx := s.backend, s.baseCtx
	s.mu.RUnlock()
	assignment, policy, ok := prepareAssignment(policies, assignments, assignment)
	if !ok {
		return policymodel.Assignment{}, nil, false, nil
	}
	audit.TenantID, audit.PolicyID = assignment.TenantID, assignment.PolicyID
	audit.PolicyVersion, audit.AssignmentID = assignment.PolicyVersion, assignment.AssignmentID
	audit = normalizePolicyAudit(audit)
	preparedCommand, err := prepareAssignmentCommand(command, assignment, policy)
	if err != nil {
		return policymodel.Assignment{}, nil, false, err
	}
	if backend != nil {
		persistedCommand, err := backend.CommitPolicyAssignment(ctxOrBackground(ctx), assignment, audit, preparedCommand)
		if err != nil {
			return policymodel.Assignment{}, nil, false, fmt.Errorf("commit policy assignment: %w", err)
		}
		preparedCommand = persistedCommand
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	assignments = upsertAssignmentSnapshot(s.Assignments, assignment)
	audits := upsertPolicyAuditSnapshot(s.PolicyAudits, audit)
	commands := append([]controlmodel.ControlCommand(nil), s.ControlCommands...)
	if preparedCommand != nil {
		commands = upsertControlCommandSnapshot(commands, *preparedCommand)
	}
	if err := s.applyPolicyStateLocked(nil, assignments, audits, commands); err != nil {
		return policymodel.Assignment{}, nil, false, fmt.Errorf("commit policy assignment: %w", err)
	}
	return assignment, preparedCommand, true, nil
}

func (s *Store) loadAssignmentState(assignment policymodel.Assignment) error {
	backend, ctx := s.backendCtx()
	if backend == nil {
		return nil
	}
	policy, ok, err := backend.GetPolicy(ctx, assignment.TenantID, assignment.PolicyID, assignment.PolicyVersion)
	if err != nil {
		return fmt.Errorf("load policy for assignment: %w", err)
	}
	assignments, err := backend.ListAssignments(ctx, assignment.TenantID, assignment.AgentID)
	if err != nil {
		return fmt.Errorf("load assignments: %w", err)
	}
	s.mu.Lock()
	if ok && policyIndex(s.Policies, policy.TenantID, policy.PolicyID, policy.Version) < 0 {
		s.Policies = append(s.Policies, policy)
	}
	for _, existing := range assignments {
		s.Assignments = upsertAssignmentSnapshot(s.Assignments, existing)
	}
	s.mu.Unlock()
	return nil
}

func prepareAssignment(policies []policymodel.Policy, assignments []policymodel.Assignment, assignment policymodel.Assignment) (policymodel.Assignment, policymodel.Policy, bool) {
	if assignment.PolicyID == "" {
		return policymodel.Assignment{}, policymodel.Policy{}, false
	}
	if assignment.TenantID == "" {
		assignment.TenantID = "default"
	}
	policy, ok := publishedPolicyFromSnapshot(policies, assignment.TenantID, assignment.PolicyID, assignment.PolicyVersion)
	if !ok {
		return policymodel.Assignment{}, policymodel.Policy{}, false
	}
	assignment.PolicyVersion = policy.Version
	if assignment.AssignmentID == "" {
		assignment.AssignmentID = assignmentKey(assignment)
	}
	now := time.Now().UTC()
	assignment.CreatedAt, assignment.UpdatedAt = now, now
	for _, existing := range assignments {
		if existing.AssignmentID == assignment.AssignmentID || sameAssignmentTarget(existing, assignment) {
			assignment.CreatedAt = existing.CreatedAt
			break
		}
	}
	return assignment, policy, true
}

func publishedPolicyFromSnapshot(policies []policymodel.Policy, tenantID, policyID string, version uint64) (policymodel.Policy, bool) {
	var latest policymodel.Policy
	var found bool
	for _, policy := range policies {
		if policy.TenantID != tenantID || policy.PolicyID != policyID || !policy.Published || (version != 0 && policy.Version != version) {
			continue
		}
		if version != 0 || !found || policy.Version > latest.Version {
			latest, found = policy, true
		}
	}
	return latest, found
}

func prepareAssignmentCommand(command *controlmodel.ControlCommand, assignment policymodel.Assignment, policy policymodel.Policy) (*controlmodel.ControlCommand, error) {
	if command == nil {
		return nil, nil
	}
	prepared := *command
	prepared.TenantID, prepared.AgentID = assignment.TenantID, assignment.AgentID
	prepared.PolicyID, prepared.PolicyVersion = policy.PolicyID, policy.Version
	if len(prepared.PayloadJSON) == 0 {
		payload, err := json.Marshal(policy.EndpointPolicy())
		if err != nil {
			return nil, fmt.Errorf("encode policy downlink payload: %w", err)
		}
		prepared.PayloadJSON = payload
	}
	prepared = controlmodel.NormalizeControlCommand(prepared)
	return &prepared, nil
}

func upsertAssignmentSnapshot(assignments []policymodel.Assignment, assignment policymodel.Assignment) []policymodel.Assignment {
	out := append([]policymodel.Assignment(nil), assignments...)
	for i, existing := range out {
		if existing.AssignmentID == assignment.AssignmentID || sameAssignmentTarget(existing, assignment) {
			out[i] = assignment
			return out
		}
	}
	return append(out, assignment)
}

func upsertPolicyAuditSnapshot(audits []policymodel.AuditRecord, audit policymodel.AuditRecord) []policymodel.AuditRecord {
	out := append([]policymodel.AuditRecord(nil), audits...)
	for i, existing := range out {
		if existing.TenantID == audit.TenantID && existing.AuditID == audit.AuditID {
			out[i] = audit
			return out
		}
	}
	return append(out, audit)
}

func (s *Store) applyPolicyStateLocked(policies []policymodel.Policy, assignments []policymodel.Assignment, audits []policymodel.AuditRecord, commands []controlmodel.ControlCommand) error {
	oldPolicies, oldAssignments := s.Policies, s.Assignments
	oldAudits, oldCommands := s.PolicyAudits, s.ControlCommands
	if policies != nil {
		s.Policies = policies
	}
	if assignments != nil {
		s.Assignments = assignments
	}
	s.PolicyAudits = audits
	if commands != nil {
		s.ControlCommands = commands
	}
	if s.backend == nil && s.path != "" {
		err := s.persistFileLocked()
		if err != nil {
			s.Policies, s.Assignments = oldPolicies, oldAssignments
			s.PolicyAudits, s.ControlCommands = oldAudits, oldCommands
			return err
		}
	}
	return nil
}
