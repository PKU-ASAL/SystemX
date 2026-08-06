package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	controlmodel "github.com/sysarmor/sysarmor-next-project/packages/contracts/controlmodel"
	responsemodel "github.com/sysarmor/sysarmor-next-project/packages/response"
)

func (s *Store) CreateResponse(cmd responsemodel.Command) (responsemodel.Command, error) {
	cmd = responsemodel.NormalizeCommand(cmd)
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.RLock()
	backend, ctx := s.backend, s.baseCtx
	if backend == nil {
		for _, existing := range s.Responses {
			if existing.TenantID == cmd.TenantID && existing.ResponseID == cmd.ResponseID {
				s.mu.RUnlock()
				return existing, nil
			}
		}
	}
	s.mu.RUnlock()
	if backend != nil {
		out, authoritative, err := s.createResponseInBackend(backend, ctxOrBackground(ctx), cmd)
		if err != nil {
			return responsemodel.Command{}, err
		}
		if authoritative {
			return out, nil
		}
	}
	s.mu.Lock()
	oldResponses := s.Responses
	s.Responses = upsertResponseSnapshot(append([]responsemodel.Command(nil), s.Responses...), cmd)
	if err := s.persistFileLocked(); err != nil {
		s.Responses = oldResponses
		s.mu.Unlock()
		return responsemodel.Command{}, fmt.Errorf("write response: %w", err)
	}
	s.mu.Unlock()
	return cmd, nil
}

func (s *Store) createResponseInBackend(backend Backend, ctx context.Context, cmd responsemodel.Command) (responsemodel.Command, bool, error) {
	records, err := backend.ListResponses(ctx, cmd.TenantID, "")
	if err != nil {
		return responsemodel.Command{}, false, fmt.Errorf("find existing response: %w", err)
	}
	if existing, ok := findResponseAudit(records, cmd.TenantID, cmd.ResponseID); ok {
		s.mu.Lock()
		s.cacheResponseAuditLocked(existing)
		s.mu.Unlock()
		return existing.Command, true, nil
	}
	created, err := backend.CreateResponse(ctx, cmd)
	if err != nil {
		return responsemodel.Command{}, false, fmt.Errorf("write response: %w", err)
	}
	if created {
		return cmd, false, nil
	}
	records, err = backend.ListResponses(ctx, cmd.TenantID, "")
	if err != nil {
		return responsemodel.Command{}, false, fmt.Errorf("reload conflicting response: %w", err)
	}
	existing, ok := findResponseAudit(records, cmd.TenantID, cmd.ResponseID)
	if !ok {
		return responsemodel.Command{}, false, fmt.Errorf("reload conflicting response: response not found")
	}
	s.mu.Lock()
	s.cacheResponseAuditLocked(existing)
	s.mu.Unlock()
	return existing.Command, true, nil
}

func findResponseAudit(records []responsemodel.AuditRecord, tenantID, responseID string) (responsemodel.AuditRecord, bool) {
	for _, record := range records {
		if record.Command.TenantID == tenantID && record.Command.ResponseID == responseID {
			return record, true
		}
	}
	return responsemodel.AuditRecord{}, false
}

func (s *Store) cacheResponseAuditLocked(record responsemodel.AuditRecord) {
	s.Responses = upsertResponseSnapshot(s.Responses, record.Command)
	if record.Ack != nil {
		s.ResponseAcks = upsertResponseAckSnapshot(s.ResponseAcks, *record.Ack)
	}
}

func upsertResponseSnapshot(responses []responsemodel.Command, command responsemodel.Command) []responsemodel.Command {
	for i, existing := range responses {
		if existing.TenantID == command.TenantID && existing.ResponseID == command.ResponseID {
			responses[i] = command
			return responses
		}
	}
	return append(responses, command)
}

func upsertResponseAckSnapshot(acks []responsemodel.Ack, ack responsemodel.Ack) []responsemodel.Ack {
	for i, existing := range acks {
		if existing.TenantID == ack.TenantID && existing.ResponseID == ack.ResponseID {
			acks[i] = ack
			return acks
		}
	}
	return append(acks, ack)
}

func (s *Store) ListResponses(tenantID, agentID string) []responsemodel.AuditRecord {
	if backend, ctx := s.backendCtx(); backend != nil {
		if records, err := backend.ListResponses(ctx, tenantID, agentID); err == nil {
			return records
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	acks := map[string]responsemodel.Ack{}
	for _, ack := range s.ResponseAcks {
		acks[responseKey(ack.TenantID, ack.ResponseID)] = ack
	}
	out := make([]responsemodel.AuditRecord, 0, len(s.Responses))
	for _, cmd := range s.Responses {
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		record := responsemodel.AuditRecord{Command: cmd}
		if ack, ok := acks[responseKey(cmd.TenantID, cmd.ResponseID)]; ok {
			record.Ack = &ack
		}
		out = append(out, record)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Command.CreatedAt.Before(out[j].Command.CreatedAt)
	})
	return out
}

func (s *Store) ListResponsesWithError(tenantID, agentID string) ([]responsemodel.AuditRecord, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		records, err := backend.ListResponses(ctx, tenantID, agentID)
		if err != nil {
			return nil, fmt.Errorf("list responses: %w", err)
		}
		return records, nil
	}
	return s.ListResponses(tenantID, agentID), nil
}

func (s *Store) PendingResponses(tenantID, agentID string) []responsemodel.Command {
	if backend, ctx := s.backendCtx(); backend != nil {
		if records, err := backend.ListResponses(ctx, tenantID, agentID); err == nil {
			return pendingResponsesFromAudit(records)
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]responsemodel.Command, 0, len(s.Responses))
	acked := map[string]bool{}
	for _, ack := range s.ResponseAcks {
		acked[responseKey(ack.TenantID, ack.ResponseID)] = true
	}
	for _, cmd := range s.Responses {
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		if cmd.Status != "pending" || acked[responseKey(cmd.TenantID, cmd.ResponseID)] {
			continue
		}
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) PendingResponsesWithError(tenantID, agentID string) ([]responsemodel.Command, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		records, err := backend.ListResponses(ctx, tenantID, agentID)
		if err != nil {
			return nil, fmt.Errorf("list pending responses: %w", err)
		}
		return pendingResponsesFromAudit(records), nil
	}
	return s.PendingResponses(tenantID, agentID), nil
}

func pendingResponsesFromAudit(records []responsemodel.AuditRecord) []responsemodel.Command {
	out := make([]responsemodel.Command, 0, len(records))
	for _, record := range records {
		if record.Command.Status == "pending" && record.Ack == nil {
			out = append(out, record.Command)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) ApproveResponse(tenantID, agentID, responseID string, approved bool, actor, role, reason string) (responsemodel.Command, bool) {
	if responseID == "" {
		return responsemodel.Command{}, false
	}
	if tenantID == "" {
		tenantID = "default"
	}
	now := time.Now().UTC()
	s.mu.Lock()
	for i, cmd := range s.Responses {
		if cmd.ResponseID != responseID {
			continue
		}
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		if !cmd.ApprovalRequired || cmd.Status != "pending_approval" || (cmd.ApprovalStatus != "required" && cmd.ApprovalStatus != "partial") {
			s.mu.Unlock()
			return responsemodel.Command{}, false
		}
		if !responsemodel.ApprovalRoleAllowed(cmd, role) {
			s.mu.Unlock()
			return responsemodel.Command{}, false
		}
		approval := responsemodel.Approval{
			Actor:      actor,
			Role:       role,
			Approved:   approved,
			Reason:     reason,
			ObservedAt: now,
		}
		cmd.Approvals = append(cmd.Approvals, approval)
		if approved && responsemodel.ApprovalCount(cmd) >= responsemodel.ApprovalThreshold(cmd) {
			cmd.Status = "pending"
			cmd.ApprovalStatus = "approved"
			cmd.ApprovedBy = actor
			cmd.ApprovedAt = now
		} else if approved {
			cmd.ApprovalStatus = "partial"
		} else {
			cmd.Status = "denied"
			cmd.ApprovalStatus = "rejected"
			cmd.ApprovedBy = actor
			cmd.ApprovedAt = now
		}
		cmd.UpdatedAt = now
		if reason != "" {
			if cmd.Reason == "" {
				cmd.Reason = reason
			} else {
				cmd.Reason = cmd.Reason + "; approval: " + reason
			}
		}
		s.Responses[i] = cmd
		backend, ctx := s.backend, s.baseCtx
		s.mu.Unlock()
		if backend != nil {
			_ = backend.WriteResponse(ctxOrBackground(ctx), cmd, nil)
		}
		return cmd, true
	}
	s.mu.Unlock()
	return responsemodel.Command{}, false
}

func (s *Store) AckResponse(ack responsemodel.Ack) (responsemodel.Command, bool, error) {
	if ack.ResponseID == "" {
		return responsemodel.Command{}, false, nil
	}
	if ack.ObservedAt.IsZero() {
		ack.ObservedAt = time.Now().UTC()
	}
	if ack.TenantID == "" {
		ack.TenantID = "default"
	}
	if ack.AgentID == "" {
		return responsemodel.Command{}, false, nil
	}
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.RLock()
	var command responsemodel.Command
	var ok bool
	for _, cmd := range s.Responses {
		if cmd.ResponseID == ack.ResponseID && cmd.TenantID == ack.TenantID && cmd.AgentID == ack.AgentID {
			command = cmd
			ok = true
			break
		}
	}
	backend, ctx := s.backend, s.baseCtx
	s.mu.RUnlock()
	if !ok {
		if backend != nil {
			records, err := backend.ListResponses(ctxOrBackground(ctx), ack.TenantID, ack.AgentID)
			if err != nil {
				return responsemodel.Command{}, false, fmt.Errorf("find response for ack: %w", err)
			}
			if record, found := findResponseAudit(records, ack.TenantID, ack.ResponseID); found {
				command, ok = record.Command, record.Command.AgentID == ack.AgentID
			}
		}
		if !ok {
			return responsemodel.Command{}, false, nil
		}
	}
	command.Status = "acked"
	command.UpdatedAt = ack.ObservedAt
	if backend != nil {
		if err := backend.WriteResponse(ctxOrBackground(ctx), command, &ack); err != nil {
			return responsemodel.Command{}, false, fmt.Errorf("write response ack: %w", err)
		}
	}
	s.mu.Lock()
	oldResponses, oldAcks := s.Responses, s.ResponseAcks
	s.Responses = upsertResponseSnapshot(append([]responsemodel.Command(nil), s.Responses...), command)
	s.ResponseAcks = upsertResponseAckSnapshot(append([]responsemodel.Ack(nil), s.ResponseAcks...), ack)
	if err := s.persistFileLocked(); err != nil {
		s.Responses, s.ResponseAcks = oldResponses, oldAcks
		s.mu.Unlock()
		return responsemodel.Command{}, false, fmt.Errorf("write response ack: %w", err)
	}
	s.mu.Unlock()
	return command, true, nil
}

func responseKey(tenantID, responseID string) string {
	return tenantID + "/" + responseID
}

func (s *Store) CreateEvidencePullback(req controlmodel.EvidencePullbackRequest) controlmodel.EvidencePullbackRequest {
	req = controlmodel.NormalizeEvidencePullback(req)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, existing := range s.Pullbacks {
		if existing.RequestID == req.RequestID {
			req.CreatedAt = existing.CreatedAt
			s.Pullbacks[i] = req
			return req
		}
	}
	s.Pullbacks = append(s.Pullbacks, req)
	return req
}

func (s *Store) ListEvidencePullbacks(tenantID, agentID string) []controlmodel.EvidencePullbackRequest {
	if backend, ctx := s.backendCtx(); backend != nil {
		if pullbacks, err := backend.ListEvidencePullbacks(ctx, tenantID, agentID); err == nil {
			return pullbacks
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]controlmodel.EvidencePullbackRequest, 0, len(s.Pullbacks))
	for _, req := range s.Pullbacks {
		if tenantID != "" && req.TenantID != tenantID {
			continue
		}
		if agentID != "" && req.AgentID != agentID {
			continue
		}
		out = append(out, req)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) ListEvidencePullbacksWithError(tenantID, agentID string) ([]controlmodel.EvidencePullbackRequest, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		pullbacks, err := backend.ListEvidencePullbacks(ctx, tenantID, agentID)
		if err != nil {
			return nil, fmt.Errorf("list evidence pullbacks: %w", err)
		}
		return pullbacks, nil
	}
	return s.ListEvidencePullbacks(tenantID, agentID), nil
}

func (s *Store) GetEvidencePullback(requestID, tenantID, agentID string) (controlmodel.EvidencePullbackRequest, bool) {
	if requestID == "" {
		return controlmodel.EvidencePullbackRequest{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, req := range s.Pullbacks {
		if req.RequestID != requestID {
			continue
		}
		if tenantID != "" && req.TenantID != tenantID {
			continue
		}
		if agentID != "" && req.AgentID != agentID {
			continue
		}
		return req, true
	}
	return controlmodel.EvidencePullbackRequest{}, false
}

func (s *Store) GetEvidencePullbackWithError(requestID, tenantID, agentID string) (controlmodel.EvidencePullbackRequest, bool, error) {
	if requestID == "" {
		return controlmodel.EvidencePullbackRequest{}, false, nil
	}
	if backend, ctx := s.backendCtx(); backend != nil {
		pullbacks, err := backend.ListEvidencePullbacks(ctx, tenantID, agentID)
		if err != nil {
			return controlmodel.EvidencePullbackRequest{}, false, fmt.Errorf("get evidence pullback: %w", err)
		}
		for _, req := range pullbacks {
			if req.RequestID == requestID {
				return req, true, nil
			}
		}
		return controlmodel.EvidencePullbackRequest{}, false, nil
	}
	req, ok := s.GetEvidencePullback(requestID, tenantID, agentID)
	return req, ok, nil
}

func (s *Store) PendingEvidencePullbacks(tenantID, agentID string) []controlmodel.EvidencePullbackRequest {
	all := s.ListEvidencePullbacks(tenantID, agentID)
	out := make([]controlmodel.EvidencePullbackRequest, 0, len(all))
	for _, req := range all {
		if req.Status == controlmodel.EvidencePullbackStatusPending {
			out = append(out, req)
		}
	}
	return out
}

func (s *Store) PendingEvidencePullbacksWithError(tenantID, agentID string) ([]controlmodel.EvidencePullbackRequest, error) {
	all, err := s.ListEvidencePullbacksWithError(tenantID, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]controlmodel.EvidencePullbackRequest, 0, len(all))
	for _, req := range all {
		if req.Status == controlmodel.EvidencePullbackStatusPending {
			out = append(out, req)
		}
	}
	return out, nil
}

func (s *Store) CompleteEvidencePullback(result controlmodel.EvidencePullbackResult) (controlmodel.EvidencePullbackRequest, bool) {
	if result.RequestID == "" {
		return controlmodel.EvidencePullbackRequest{}, false
	}
	if result.ObservedAt.IsZero() {
		result.ObservedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, req := range s.Pullbacks {
		if req.RequestID != result.RequestID {
			continue
		}
		if result.TenantID != "" && req.TenantID != result.TenantID {
			continue
		}
		if result.AgentID != "" && req.AgentID != result.AgentID {
			continue
		}
		req.ResultOK = result.OK
		req.Result = result.Message
		req.UpdatedAt = result.ObservedAt
		req.CompletedAt = result.ObservedAt
		if result.OK {
			req.Status = controlmodel.EvidencePullbackStatusCompleted
		} else {
			req.Status = controlmodel.EvidencePullbackStatusFailed
		}
		s.Pullbacks[i] = req
		return req, true
	}
	return controlmodel.EvidencePullbackRequest{}, false
}

func (s *Store) CreateControlCommand(cmd controlmodel.ControlCommand) (controlmodel.ControlCommand, error) {
	cmd = controlmodel.NormalizeControlCommand(cmd)
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.RLock()
	backend, ctx := s.backend, s.baseCtx
	if backend == nil {
		for _, existing := range s.ControlCommands {
			if existing.CommandID == cmd.CommandID && existing.TenantID == cmd.TenantID {
				s.mu.RUnlock()
				return existing, nil
			}
		}
	}
	s.mu.RUnlock()
	if backend != nil {
		out, authoritative, err := s.createControlCommandInBackend(backend, ctxOrBackground(ctx), cmd)
		if err != nil {
			return controlmodel.ControlCommand{}, err
		}
		if authoritative {
			return out, nil
		}
	}
	s.mu.Lock()
	oldCommands := s.ControlCommands
	s.ControlCommands = upsertControlCommandSnapshot(append([]controlmodel.ControlCommand(nil), s.ControlCommands...), cmd)
	if err := s.persistFileLocked(); err != nil {
		s.ControlCommands = oldCommands
		s.mu.Unlock()
		return controlmodel.ControlCommand{}, fmt.Errorf("write control command: %w", err)
	}
	s.mu.Unlock()
	return cmd, nil
}

func (s *Store) createControlCommandInBackend(backend Backend, ctx context.Context, cmd controlmodel.ControlCommand) (controlmodel.ControlCommand, bool, error) {
	commands, err := backend.ListControlCommands(ctx, cmd.TenantID, "", "")
	if err != nil {
		return controlmodel.ControlCommand{}, false, fmt.Errorf("find existing control command: %w", err)
	}
	if existing, ok := findControlCommandByTenant(commands, cmd.TenantID, cmd.CommandID); ok {
		s.cacheControlCommand(existing)
		return existing, true, nil
	}
	created, err := backend.CreateControlCommand(ctx, cmd)
	if err != nil {
		return controlmodel.ControlCommand{}, false, fmt.Errorf("write control command: %w", err)
	}
	if created {
		return cmd, false, nil
	}
	commands, err = backend.ListControlCommands(ctx, cmd.TenantID, "", "")
	if err != nil {
		return controlmodel.ControlCommand{}, false, fmt.Errorf("reload conflicting control command: %w", err)
	}
	existing, ok := findControlCommandByTenant(commands, cmd.TenantID, cmd.CommandID)
	if !ok {
		return controlmodel.ControlCommand{}, false, fmt.Errorf("reload conflicting control command: command not found")
	}
	s.cacheControlCommand(existing)
	return existing, true, nil
}

func (s *Store) cacheControlCommand(cmd controlmodel.ControlCommand) {
	s.mu.Lock()
	s.ControlCommands = upsertControlCommandSnapshot(s.ControlCommands, cmd)
	s.mu.Unlock()
}

func (s *Store) ListControlCommands(tenantID, agentID, commandType string) []controlmodel.ControlCommand {
	if backend, ctx := s.backendCtx(); backend != nil {
		if commands, err := backend.ListControlCommands(ctx, tenantID, agentID, commandType); err == nil {
			return commands
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]controlmodel.ControlCommand, 0, len(s.ControlCommands))
	for _, cmd := range s.ControlCommands {
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		if commandType != "" && cmd.Type != commandType {
			continue
		}
		out = append(out, cmd)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (s *Store) ListControlCommandsWithError(tenantID, agentID, commandType string) ([]controlmodel.ControlCommand, error) {
	if backend, ctx := s.backendCtx(); backend != nil {
		commands, err := backend.ListControlCommands(ctx, tenantID, agentID, commandType)
		if err != nil {
			return nil, fmt.Errorf("list control commands: %w", err)
		}
		return commands, nil
	}
	return s.ListControlCommands(tenantID, agentID, commandType), nil
}

func (s *Store) PendingControlCommands(tenantID, agentID string) []controlmodel.ControlCommand {
	all := s.ListControlCommands(tenantID, agentID, "")
	out := make([]controlmodel.ControlCommand, 0, len(all))
	for _, cmd := range all {
		if controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			continue
		}
		out = append(out, cmd)
	}
	return out
}

func (s *Store) PendingControlCommandsWithError(tenantID, agentID string) ([]controlmodel.ControlCommand, error) {
	all, err := s.ListControlCommandsWithError(tenantID, agentID, "")
	if err != nil {
		return nil, err
	}
	out := make([]controlmodel.ControlCommand, 0, len(all))
	for _, cmd := range all {
		if !controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			out = append(out, cmd)
		}
	}
	return out, nil
}

func (s *Store) MarkControlCommandSent(commandID, tenantID, agentID string, sentAt time.Time) (controlmodel.ControlCommand, bool) {
	if commandID == "" {
		return controlmodel.ControlCommand{}, false
	}
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	s.mu.Lock()
	for i, cmd := range s.ControlCommands {
		if cmd.CommandID != commandID {
			continue
		}
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		if !controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			cmd.Status = controlmodel.ControlCommandStatusSent
		}
		cmd.SentAt = sentAt
		cmd.LastSentAt = sentAt
		cmd.AttemptCount++
		cmd.UpdatedAt = sentAt
		s.ControlCommands[i] = cmd
		s.mu.Unlock()
		return cmd, true
	}
	s.mu.Unlock()
	for _, cmd := range s.ListControlCommands(tenantID, agentID, "") {
		if cmd.CommandID != commandID {
			continue
		}
		if !controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			cmd.Status = controlmodel.ControlCommandStatusSent
		}
		cmd.SentAt = sentAt
		cmd.LastSentAt = sentAt
		cmd.AttemptCount++
		cmd.UpdatedAt = sentAt
		s.mu.Lock()
		s.ControlCommands = upsertControlCommandSnapshot(s.ControlCommands, cmd)
		s.mu.Unlock()
		return cmd, true
	}
	return controlmodel.ControlCommand{}, false
}

func (s *Store) CancelControlCommand(commandID, tenantID, agentID, actor, reason string) (controlmodel.ControlCommand, bool) {
	if commandID == "" {
		return controlmodel.ControlCommand{}, false
	}
	now := time.Now().UTC()
	return s.updateControlCommand(commandID, tenantID, agentID, func(cmd controlmodel.ControlCommand) controlmodel.ControlCommand {
		if controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			return cmd
		}
		cmd.Status = controlmodel.ControlCommandStatusCanceled
		cmd.CanceledAt = now
		cmd.UpdatedAt = now
		if actor != "" {
			cmd.Actor = actor
		}
		if reason != "" {
			cmd.Error = reason
		}
		return cmd
	})
}

func (s *Store) RetryControlCommand(commandID, tenantID, agentID, actor, reason string) (controlmodel.ControlCommand, bool) {
	if commandID == "" {
		return controlmodel.ControlCommand{}, false
	}
	now := time.Now().UTC()
	return s.updateControlCommand(commandID, tenantID, agentID, func(cmd controlmodel.ControlCommand) controlmodel.ControlCommand {
		if cmd.Status == controlmodel.ControlCommandStatusApplied {
			return cmd
		}
		cmd.Status = controlmodel.ControlCommandStatusPending
		cmd.UpdatedAt = now
		cmd.AckedAt = time.Time{}
		cmd.CanceledAt = time.Time{}
		cmd.ExpiredAt = time.Time{}
		cmd.AckStatus = ""
		cmd.AckMessage = ""
		cmd.AckPolicyID = ""
		cmd.AckPolicyVer = 0
		cmd.AckReportJSON = ""
		cmd.Error = ""
		if actor != "" {
			cmd.Actor = actor
		}
		if reason != "" {
			cmd.Reason = reason
		}
		return cmd
	})
}

func (s *Store) ExpireControlCommand(commandID, tenantID, agentID, reason string) (controlmodel.ControlCommand, bool) {
	if commandID == "" {
		return controlmodel.ControlCommand{}, false
	}
	now := time.Now().UTC()
	return s.updateControlCommand(commandID, tenantID, agentID, func(cmd controlmodel.ControlCommand) controlmodel.ControlCommand {
		if controlmodel.ControlCommandTerminalStatus(cmd.Status) {
			return cmd
		}
		cmd.Status = controlmodel.ControlCommandStatusExpired
		cmd.ExpiredAt = now
		cmd.UpdatedAt = now
		if reason != "" {
			cmd.Error = reason
		}
		return cmd
	})
}

func (s *Store) updateControlCommand(commandID, tenantID, agentID string, update func(controlmodel.ControlCommand) controlmodel.ControlCommand) (controlmodel.ControlCommand, bool) {
	s.mu.Lock()
	for i, cmd := range s.ControlCommands {
		if cmd.CommandID != commandID {
			continue
		}
		if tenantID != "" && cmd.TenantID != tenantID {
			continue
		}
		if agentID != "" && cmd.AgentID != agentID {
			continue
		}
		cmd = update(cmd)
		s.ControlCommands[i] = cmd
		s.mu.Unlock()
		return cmd, true
	}
	s.mu.Unlock()
	for _, cmd := range s.ListControlCommands(tenantID, agentID, "") {
		if cmd.CommandID != commandID {
			continue
		}
		cmd = update(cmd)
		s.mu.Lock()
		s.ControlCommands = upsertControlCommandSnapshot(s.ControlCommands, cmd)
		s.mu.Unlock()
		return cmd, true
	}
	return controlmodel.ControlCommand{}, false
}

func (s *Store) AckControlCommand(ack controlmodel.ControlCommandAck) (controlmodel.ControlCommand, bool, error) {
	if ack.CommandID == "" {
		return controlmodel.ControlCommand{}, false, nil
	}
	if ack.ObservedAt.IsZero() {
		ack.ObservedAt = time.Now().UTC()
	}
	if ack.TenantID == "" {
		ack.TenantID = "default"
	}
	if ack.AgentID == "" {
		return controlmodel.ControlCommand{}, false, nil
	}
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.mu.RLock()
	cmd, ok := findControlCommand(s.ControlCommands, ack.TenantID, ack.AgentID, ack.CommandID)
	backend, ctx := s.backend, s.baseCtx
	s.mu.RUnlock()
	if !ok && backend != nil {
		commands, err := backend.ListControlCommands(ctxOrBackground(ctx), ack.TenantID, ack.AgentID, "")
		if err != nil {
			return controlmodel.ControlCommand{}, false, fmt.Errorf("find control command for ack: %w", err)
		}
		cmd, ok = findControlCommand(commands, ack.TenantID, ack.AgentID, ack.CommandID)
	}
	if !ok {
		return controlmodel.ControlCommand{}, false, nil
	}
	cmd = applyControlCommandAck(cmd, ack)
	if backend != nil {
		if err := backend.WriteControlCommand(ctxOrBackground(ctx), cmd); err != nil {
			return controlmodel.ControlCommand{}, false, fmt.Errorf("write control command ack: %w", err)
		}
	}
	s.mu.Lock()
	oldCommands := s.ControlCommands
	s.ControlCommands = upsertControlCommandSnapshot(append([]controlmodel.ControlCommand(nil), s.ControlCommands...), cmd)
	if err := s.persistFileLocked(); err != nil {
		s.ControlCommands = oldCommands
		s.mu.Unlock()
		return controlmodel.ControlCommand{}, false, fmt.Errorf("write control command ack: %w", err)
	}
	s.mu.Unlock()
	return cmd, true, nil
}

func findControlCommand(commands []controlmodel.ControlCommand, tenantID, agentID, commandID string) (controlmodel.ControlCommand, bool) {
	for _, cmd := range commands {
		if cmd.CommandID == commandID && cmd.TenantID == tenantID && cmd.AgentID == agentID {
			return cmd, true
		}
	}
	return controlmodel.ControlCommand{}, false
}

func findControlCommandByTenant(commands []controlmodel.ControlCommand, tenantID, commandID string) (controlmodel.ControlCommand, bool) {
	for _, cmd := range commands {
		if cmd.CommandID == commandID && cmd.TenantID == tenantID {
			return cmd, true
		}
	}
	return controlmodel.ControlCommand{}, false
}

func applyControlCommandAck(cmd controlmodel.ControlCommand, ack controlmodel.ControlCommandAck) controlmodel.ControlCommand {
	status := ack.Status
	if status == "" {
		status = controlmodel.ControlCommandStatusApplied
	}
	switch status {
	case controlmodel.ControlCommandStatusApplied, "accepted", "validated", "degraded":
		cmd.Status = controlmodel.ControlCommandStatusApplied
	case controlmodel.ControlCommandStatusRejected:
		cmd.Status = controlmodel.ControlCommandStatusRejected
		cmd.Error = ack.Message
	case controlmodel.ControlCommandStatusFailed:
		cmd.Status = controlmodel.ControlCommandStatusFailed
		cmd.Error = ack.Message
	default:
		cmd.Status = status
	}
	cmd.AckStatus = ack.Status
	cmd.AckMessage = ack.Message
	cmd.AckPolicyID = ack.PolicyID
	cmd.AckPolicyVer = ack.PolicyVersion
	cmd.AckReportJSON = ack.ReportJSON
	cmd.AckedAt = ack.ObservedAt
	cmd.UpdatedAt = ack.ObservedAt
	return cmd
}

func upsertControlCommandSnapshot(commands []controlmodel.ControlCommand, cmd controlmodel.ControlCommand) []controlmodel.ControlCommand {
	for i, existing := range commands {
		if existing.CommandID == cmd.CommandID && existing.TenantID == cmd.TenantID {
			commands[i] = cmd
			return commands
		}
	}
	return append(commands, cmd)
}
