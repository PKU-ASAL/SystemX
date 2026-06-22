package agentplane

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	incidentv1 "github.com/sysarmor/sysarmor-next-project/api/proto/incident/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	gatewaymodel "github.com/sysarmor/sysarmor-next-project/internal/agentplane/model"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type controlGRPCServer struct {
	controlv1.UnimplementedAgentControlServiceServer
	backend Backend
}

func NewControlServer(backend Backend) controlv1.AgentControlServiceServer {
	return &controlGRPCServer{backend: backend}
}

func (s *controlGRPCServer) ControlStream(stream controlv1.AgentControlService_ControlStreamServer) error {
	if !s.authorized(stream.Context()) {
		return status.Error(codes.Unauthenticated, "unauthorized")
	}
	state := controlStreamState{nextIncoming: 1, nextOutgoing: 1}
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "recv control stream frame: %v", err)
		}
		if err := state.acceptIncoming(frame); err != nil {
			out := []*controlv1.ControlStreamFrame{controlAckFrame(frame, "rejected", err.Error(), errorCode(err), errorRetryable(err))}
			state.assignOutgoing(out)
			for _, reply := range out {
				if err := stream.Send(reply); err != nil {
					return status.Errorf(codes.Internal, "send control stream frame: %v", err)
				}
			}
			continue
		}
		out, err := s.acceptControlFrames(stream.Context(), frame)
		if err != nil {
			out = []*controlv1.ControlStreamFrame{controlAckFrame(frame, "rejected", err.Error(), errorCode(err), errorRetryable(err))}
		}
		state.assignOutgoing(out)
		for _, reply := range out {
			if err := stream.Send(reply); err != nil {
				return status.Errorf(codes.Internal, "send control stream frame: %v", err)
			}
		}
	}
}

type controlStreamState struct {
	nextIncoming uint64
	nextOutgoing uint64
}

func (s *controlStreamState) acceptIncoming(frame *controlv1.ControlStreamFrame) error {
	if frame == nil {
		return status.Error(codes.InvalidArgument, "control stream frame is nil")
	}
	if frame.GetContractVersion() != 1 {
		return status.Errorf(codes.InvalidArgument, "unsupported control contract_version %d", frame.GetContractVersion())
	}
	seq := frame.GetSequence()
	if seq == 0 {
		return status.Error(codes.InvalidArgument, "control stream sequence is required")
	}
	if seq < s.nextIncoming {
		return status.Errorf(codes.AlreadyExists, "control stream replay sequence %d; expected %d", seq, s.nextIncoming)
	}
	if seq > s.nextIncoming {
		return status.Errorf(codes.FailedPrecondition, "control stream sequence gap: got %d; expected %d", seq, s.nextIncoming)
	}
	s.nextIncoming++
	return nil
}

func (s *controlStreamState) assignOutgoing(frames []*controlv1.ControlStreamFrame) {
	for _, frame := range frames {
		if frame == nil {
			continue
		}
		frame.ContractVersion = 1
		frame.Sequence = s.nextOutgoing
		s.nextOutgoing++
	}
}

func (s *controlGRPCServer) acceptControlFrames(ctx context.Context, frame *controlv1.ControlStreamFrame) ([]*controlv1.ControlStreamFrame, error) {
	if frame == nil {
		return nil, status.Error(codes.InvalidArgument, "control stream frame is nil")
	}
	peerID, hasPeer, err := validatePeerControlIdentity(ctx, frame.GetContext())
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}
	if hasPeer {
		reqCtx := frame.GetContext()
		version := ""
		hostID := ""
		switch frame.GetType() {
		case "health_report":
			hostID = frame.GetHealth().GetHostId()
			version = frame.GetHealth().GetCapability().GetVersion()
		case "capability_report":
			hostID = frame.GetCapability().GetHostId()
			version = frame.GetCapability().GetSensor().GetVersion()
		}
		if reqCtx.GetAgentId() != "" {
			agent := agentIdentityFromPeer(peerID, hostID, version)
			if err := s.backend.BindAgentIdentity(agent); err != nil {
				return nil, status.Error(codes.PermissionDenied, err.Error())
			}
		}
	}
	switch frame.GetType() {
	case "hello":
		ctx := frame.GetContext()
		if ctx.GetAgentId() == "" {
			return nil, status.Error(codes.InvalidArgument, "hello agent_id is required")
		}
		tenantID := ctx.GetTenantId()
		if tenantID == "" {
			tenantID = "default"
		}
		scope := ctx.GetScope()
		st := s.backend.Store()
		policy, _ := st.EffectivePolicy(tenantID, ctx.GetAgentId(), scope.GetType(), scope.GetSelector())
		st.AddAgent(store.AgentIdentity{AgentID: ctx.GetAgentId(), TenantID: tenantID})
		session := st.RecordControlSessionOpen(tenantID, ctx.GetAgentId(), "control", time.Now().UTC())
		s.backend.TouchHotSession(session)
		replies := []*controlv1.ControlStreamFrame{{
			Type:            "policy_update",
			RequestId:       frame.GetRequestId(),
			Context:         &controlv1.RequestContext{TenantId: tenantID, AgentId: ctx.GetAgentId(), Scope: ctx.GetScope()},
			ContractVersion: 1,
			PolicyUpdate:    currentPolicyFrame(policy),
		}, {
			Type:            "resume",
			RequestId:       frame.GetRequestId(),
			Context:         &controlv1.RequestContext{TenantId: tenantID, AgentId: ctx.GetAgentId(), Scope: ctx.GetScope()},
			ContractVersion: 1,
			Resume:          resumeCursorFrame(s.backend.ResumeCursor(tenantID, ctx.GetAgentId())),
		}}
		for _, cmd := range st.PendingResponses(tenantID, ctx.GetAgentId()) {
			replies = append(replies, &controlv1.ControlStreamFrame{
				Type:            "response_command",
				RequestId:       frame.GetRequestId(),
				Context:         &controlv1.RequestContext{TenantId: tenantID, AgentId: ctx.GetAgentId(), Scope: ctx.GetScope()},
				ContractVersion: 1,
				ResponseCommand: responseCommandControlFrame(cmd),
			})
		}
		for _, req := range st.PendingEvidencePullbacks(tenantID, ctx.GetAgentId()) {
			replies = append(replies, &controlv1.ControlStreamFrame{
				Type:             "evidence_pullback",
				RequestId:        frame.GetRequestId(),
				Context:          &controlv1.RequestContext{TenantId: tenantID, AgentId: ctx.GetAgentId(), Scope: ctx.GetScope()},
				ContractVersion:  1,
				EvidencePullback: evidencePullbackControlFrame(req),
			})
		}
		return replies, nil
	case "health_report":
		health := agentHealthFromControl(frame.GetHealth())
		if health.AgentID == "" {
			return nil, status.Error(codes.InvalidArgument, "health_report agent_id is required")
		}
		st := s.backend.Store()
		st.UpsertAgentHealth(health)
		st.AddAgent(store.AgentIdentity{AgentID: health.AgentID, HostID: health.HostID, TenantID: health.TenantID, Version: health.Capability.Version})
		if err := st.Save(); err != nil {
			return nil, status.Errorf(codes.Internal, "save health: %v", err)
		}
		return []*controlv1.ControlStreamFrame{controlAckFrame(frame, "accepted", "health accepted", "", false)}, nil
	case "capability_report":
		cap := frame.GetCapability()
		if cap.GetAgentId() == "" {
			return nil, status.Error(codes.InvalidArgument, "capability_report agent_id is required")
		}
		st := s.backend.Store()
		st.AddAgent(store.AgentIdentity{AgentID: cap.GetAgentId(), HostID: cap.GetHostId(), TenantID: cap.GetTenantId(), Version: cap.GetSensor().GetVersion()})
		if err := st.Save(); err != nil {
			return nil, status.Errorf(codes.Internal, "save capability: %v", err)
		}
		return []*controlv1.ControlStreamFrame{controlAckFrame(frame, "accepted", "capability accepted", "", false)}, nil
	case "response_ack":
		ack := responseAckFromControl(frame.GetResponseAck())
		if ack.ResponseID == "" {
			return nil, status.Error(codes.InvalidArgument, "response_ack response_id is required")
		}
		st := s.backend.Store()
		if _, ok := st.AckResponse(ack); !ok {
			return nil, status.Error(codes.NotFound, "response command not found")
		}
		if err := st.Save(); err != nil {
			return nil, status.Errorf(codes.Internal, "save response ack: %v", err)
		}
		return []*controlv1.ControlStreamFrame{controlAckFrame(frame, "accepted", "response ack accepted", "", false)}, nil
	case "evidence_pullback_result":
		result := evidencePullbackResultFromControl(frame.GetEvidenceResult())
		if result.RequestID == "" {
			return nil, status.Error(codes.InvalidArgument, "evidence_pullback_result request_id is required")
		}
		st := s.backend.Store()
		req, ok := st.GetEvidencePullback(result.RequestID, result.TenantID, result.AgentID)
		if !ok {
			return nil, status.Error(codes.NotFound, "evidence pullback request not found")
		}
		if len(result.Evidence) > 0 {
			evidence := &incidentv1.EvidenceSubgraph{}
			if err := protojson.Unmarshal(result.Evidence, evidence); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "decode evidence pullback evidence: %v", err)
			}
			if _, ok := st.AttachIncidentEvidence(req.IncidentID, req.Scenario, evidence); !ok {
				return nil, status.Error(codes.NotFound, "incident for evidence pullback not found")
			}
		}
		if _, ok := st.CompleteEvidencePullback(result); !ok {
			return nil, status.Error(codes.NotFound, "evidence pullback request not found")
		}
		if err := st.Save(); err != nil {
			return nil, status.Errorf(codes.Internal, "save evidence pullback result: %v", err)
		}
		return []*controlv1.ControlStreamFrame{controlAckFrame(frame, "accepted", "evidence pullback result accepted", "", false)}, nil
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported control stream frame type %q", frame.GetType())
	}
}

func currentPolicyFrame(policy policymodel.Policy) *controlv1.CurrentPolicyResponse {
	raw, _ := json.Marshal(policy)
	return &controlv1.CurrentPolicyResponse{
		PolicyId:      policy.PolicyID,
		Version:       policy.Version,
		TenantId:      policy.TenantID,
		Scope:         &controlv1.Scope{Type: policy.Scope.Type, Selector: policy.Scope.Selector},
		Mode:          policy.Mode,
		EndpointRules: append([]string(nil), policy.EndpointRules...),
		CloudRules:    append([]string(nil), policy.CloudRules...),
		Published:     policy.Published,
		RawJson:       string(raw),
	}
}

func resumeCursorFrame(cursor ResumeCursor) *controlv1.ResumeCursor {
	return &controlv1.ResumeCursor{
		TenantId:     cursor.TenantID,
		AgentId:      cursor.AgentID,
		SessionId:    cursor.SessionID,
		ResumeCursor: cursor.ResumeCursor,
	}
}

func responseCommandControlFrame(cmd responsemodel.Command) *controlv1.ResponseCommand {
	raw, _ := json.Marshal(cmd)
	return &controlv1.ResponseCommand{
		ResponseId:        cmd.ResponseID,
		TenantId:          cmd.TenantID,
		AgentId:           cmd.AgentID,
		PolicyId:          cmd.PolicyID,
		PolicyVersion:     cmd.PolicyVersion,
		SignalId:          cmd.SignalID,
		Scenario:          cmd.Scenario,
		Scope:             &controlv1.ResponseScope{Type: cmd.Scope.Type, Selector: cmd.Scope.Selector},
		Action:            cmd.Action,
		Mode:              cmd.Mode,
		Target:            cmd.Target,
		Reason:            cmd.Reason,
		Status:            cmd.Status,
		Actor:             cmd.Actor,
		ApprovalRequired:  cmd.ApprovalRequired,
		ApprovalStatus:    cmd.ApprovalStatus,
		ApprovalThreshold: cmd.ApprovalThreshold,
		ApprovalRoles:     append([]string(nil), cmd.ApprovalRoles...),
		RawJson:           string(raw),
	}
}

func evidencePullbackControlFrame(req gatewaymodel.EvidencePullbackRequest) *controlv1.EvidencePullbackRequest {
	raw, _ := json.Marshal(req)
	return &controlv1.EvidencePullbackRequest{
		RequestId:  req.RequestID,
		TenantId:   req.TenantID,
		AgentId:    req.AgentID,
		IncidentId: req.IncidentID,
		Scenario:   req.Scenario,
		Target:     req.Target,
		Reason:     req.Reason,
		Status:     req.Status,
		Actor:      req.Actor,
		RawJson:    string(raw),
	}
}

func responseAckFromControl(in *controlv1.ResponseAck) responsemodel.Ack {
	if in == nil {
		return responsemodel.Ack{}
	}
	return responsemodel.Ack{
		ResponseID:  in.GetResponseId(),
		TenantID:    in.GetTenantId(),
		AgentID:     in.GetAgentId(),
		Accepted:    in.GetAccepted(),
		Unsupported: in.GetUnsupported(),
		ObserveOnly: in.GetObserveOnly(),
		Executed:    in.GetExecuted(),
		Message:     in.GetMessage(),
		ObservedAt:  parseControlTime(in.GetObservedAt()),
	}
}

func evidencePullbackResultFromControl(in *controlv1.EvidencePullbackResult) gatewaymodel.EvidencePullbackResult {
	if in == nil {
		return gatewaymodel.EvidencePullbackResult{}
	}
	return gatewaymodel.EvidencePullbackResult{
		RequestID:  in.GetRequestId(),
		TenantID:   in.GetTenantId(),
		AgentID:    in.GetAgentId(),
		OK:         in.GetOk(),
		Message:    in.GetMessage(),
		Evidence:   append([]byte(nil), in.GetEvidenceJson()...),
		ObservedAt: parseControlTime(in.GetObservedAt()),
	}
}

func controlAckFrame(frame *controlv1.ControlStreamFrame, statusText, message, code string, retryable bool) *controlv1.ControlStreamFrame {
	req := frame.GetContext()
	out := &controlv1.ControlStreamFrame{
		Type:            "ack",
		RequestId:       frame.GetRequestId(),
		Context:         req,
		ContractVersion: 1,
		Ack: &controlv1.ControlAck{
			RequestId: frame.GetRequestId(),
			TenantId:  req.GetTenantId(),
			AgentId:   req.GetAgentId(),
			Status:    statusText,
			Message:   message,
		},
	}
	if code != "" {
		out.Error = &controlv1.ControlError{Code: code, Message: message, Retryable: retryable}
	}
	return out
}

func errorCode(err error) string {
	if st, ok := status.FromError(err); ok {
		return st.Code().String()
	}
	return codes.Unknown.String()
}

func errorRetryable(err error) bool {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded:
			return true
		default:
			return false
		}
	}
	return false
}

func (s *controlGRPCServer) authorized(ctx context.Context) bool {
	token := s.backend.AgentToken()
	if token == "" {
		return true
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	for _, value := range md.Get("x-sysarmor-agent-token") {
		if value == token {
			return true
		}
	}
	for _, value := range md.Get("authorization") {
		if value == "Bearer "+token {
			return true
		}
	}
	return false
}

func agentHealthFromControl(in *controlv1.HealthResponse) agenthealth.AgentHealth {
	if in == nil {
		return agenthealth.AgentHealth{}
	}
	return agenthealth.AgentHealth{
		AgentID:       in.GetAgentId(),
		HostID:        in.GetHostId(),
		TenantID:      in.GetTenantId(),
		Scope:         agenthealth.RuntimeScope{Type: in.GetScope().GetType(), Selector: in.GetScope().GetSelector()},
		Status:        in.GetStatus(),
		PolicyID:      in.GetPolicyId(),
		PolicyVersion: in.GetPolicyVersion(),
		PolicyMode:    in.GetPolicyMode(),
		UptimeSeconds: in.GetUptimeSeconds(),
		Capability:    sensorCapabilityFromControl(in.GetCapability()),
		Sensor: agenthealth.SensorHealth{
			Backend:        in.GetSensor().GetBackend(),
			Installed:      in.GetSensor().GetInstalled(),
			Running:        in.GetSensor().GetRunning(),
			Version:        in.GetSensor().GetVersion(),
			PolicyLoaded:   in.GetSensor().GetPolicyLoaded(),
			EventsSeen:     in.GetSensor().GetEventsSeen(),
			EventsDropped:  in.GetSensor().GetEventsDropped(),
			ParseErrors:    in.GetSensor().GetParseErrors(),
			RestartCount:   in.GetSensor().GetRestartCount(),
			LastEventAt:    parseControlTime(in.GetSensor().GetLastEventAt()),
			LastExitReason: in.GetSensor().GetLastExitReason(),
			LastError:      in.GetSensor().GetLastError(),
		},
		Queue: agenthealth.QueueHealth{
			QueuedBatches:     int(in.GetQueue().GetQueuedBatches()),
			QueuedBytes:       in.GetQueue().GetQueuedBytes(),
			MaxBytes:          in.GetQueue().GetMaxBytes(),
			BackpressureCount: in.GetQueue().GetBackpressureCount(),
			DroppedBatches:    in.GetQueue().GetDroppedBatches(),
			DroppedBytes:      in.GetQueue().GetDroppedBytes(),
			LastError:         in.GetQueue().GetLastError(),
		},
		WAL: agenthealth.WALHealth{
			QueuedBatches:     int(in.GetWal().GetQueuedBatches()),
			QueuedBytes:       in.GetWal().GetQueuedBytes(),
			MaxBytes:          in.GetWal().GetMaxBytes(),
			OldestBatchID:     in.GetWal().GetOldestBatchId(),
			NewestBatchID:     in.GetWal().GetNewestBatchId(),
			LastAckedBatchID:  in.GetWal().GetLastAckedBatchId(),
			WatchSubscribers:  in.GetWal().GetWatchSubscribers(),
			BackpressureCount: in.GetWal().GetBackpressureCount(),
			DroppedBatches:    in.GetWal().GetDroppedBatches(),
			DroppedBytes:      in.GetWal().GetDroppedBytes(),
			LastError:         in.GetWal().GetLastError(),
		},
		Upload: agenthealth.UploadHealth{
			UploadedBatches:  int(in.GetUpload().GetUploadedBatches()),
			RemainingBatches: int(in.GetUpload().GetRemainingBatches()),
			RemainingBytes:   in.GetUpload().GetRemainingBytes(),
			LastError:        in.GetUpload().GetLastError(),
		},
		CEP: agenthealth.CEPHealth{
			ActiveGroups:     in.GetCep().GetActiveGroups(),
			EvictedGroups:    in.GetCep().GetEvictedGroups(),
			ExpiredGroups:    in.GetCep().GetExpiredGroups(),
			DroppedEventRefs: in.GetCep().GetDroppedEventRefs(),
			EvalErrors:       in.GetCep().GetEvalErrors(),
			EmittedSignals:   in.GetCep().GetEmittedSignals(),
			Degraded:         in.GetCep().GetDegraded(),
		},
		ObservedAt: parseControlTime(in.GetObservedAt()),
	}
}

func sensorCapabilityFromControl(in *controlv1.SensorCapability) agenthealth.SensorCapability {
	if in == nil {
		return agenthealth.SensorCapability{}
	}
	return agenthealth.SensorCapability{
		Backend:         in.GetBackend(),
		Version:         in.GetVersion(),
		SupportsExec:    in.GetSupportsExec(),
		SupportsConnect: in.GetSupportsConnect(),
		SupportsFile:    in.GetSupportsFile(),
		SupportsEnforce: in.GetSupportsEnforce(),
		SupportsHealth:  in.GetSupportsHealth(),
		KernelRelease:   in.GetKernelRelease(),
		BTFAvailable:    in.GetBtfAvailable(),
		BPFFSAvailable:  in.GetBpffsAvailable(),
	}
}

func parseControlTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}
