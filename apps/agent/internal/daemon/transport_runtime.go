package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	agentcontrol "github.com/sysarmor/sysarmor-next-project/apps/agent/internal/control"
	"github.com/sysarmor/sysarmor-next-project/apps/agent/internal/localstore"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/packages/contracts/proto/controlplane/v1"
	"github.com/sysarmor/sysarmor-next-project/packages/tlsconfig"
	"google.golang.org/protobuf/proto"
)

func (r *TransportRuntime) runControlFlow(ctx context.Context) {
	runner := r.runner
	backoff := runner.Config.Local.Export.RetryInitial
	if backoff <= 0 {
		backoff = time.Second
	}
	maxBackoff := runner.Config.Local.Export.RetryMax
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}
	for {
		if err := r.RunControlChannel(ctx); err != nil && ctx.Err() == nil && runner.Out != nil {
			fmt.Fprintf(runner.Out, "agent control channel disconnected: %v\n", err)
		}
		if ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (r *TransportRuntime) RunControlChannel(ctx context.Context) error {
	runner := r.runner
	identity := runner.currentIdentity()
	return r.runControlChannel(ctx, runner.Config.Manager.Address, runner.Config.Agent.Token, runner.managerTLS(), identity)
}

func (r *TransportRuntime) runControlChannel(ctx context.Context, manager, token string, tlsCfg tlsconfig.ClientConfig, identity runtimeIdentity) error {
	runner := r.runner
	connectCtx, cancel := context.WithTimeout(ctx, runner.Config.Local.Export.RequestTimeout)
	defer cancel()
	session := NewControlChannel(manager, token, tlsCfg)
	if err := session.open(connectCtx, ctx); err != nil {
		return err
	}
	defer session.Close()
	frames, err := session.Hello(connectCtx, identity.TenantID, identity.AgentID, r.scopeType, r.scopeSelector)
	if err != nil {
		return err
	}
	for _, frame := range frames {
		if err := r.handleControlFrame(ctx, session, identity, frame); err != nil {
			return err
		}
	}
	health, err := runner.collectHealth(ctx, r.sensor, r.bus, r.batcher, r.sender, r.startedAt)
	if err == nil {
		health = bindHealthToSession(health, identity)
		if err := session.SendHealth(ctx, health); err != nil {
			return err
		}
		if err := session.SendCapability(ctx, health); err != nil {
			return err
		}
	}
	recvCh := make(chan *controlplanev1.ControlFrame, 1)
	errCh := make(chan error, 1)
	go func() {
		for {
			frame, err := session.Recv()
			if err != nil {
				errCh <- err
				return
			}
			select {
			case recvCh <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	interval := runner.Config.Health.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if errors.Is(err, io.EOF) {
				return fmt.Errorf("control channel closed")
			}
			return err
		case frame := <-recvCh:
			if err := r.handleControlFrame(ctx, session, identity, frame); err != nil {
				return err
			}
		case <-ticker.C:
			health, err := runner.collectHealth(ctx, r.sensor, r.bus, r.batcher, r.sender, r.startedAt)
			if err != nil {
				return err
			}
			health = bindHealthToSession(health, identity)
			if err := session.SendHealth(ctx, health); err != nil {
				return err
			}
			if err := session.SendCapability(ctx, health); err != nil {
				return err
			}
		}
	}
}

func (r *TransportRuntime) runControlFlowForEnrollment(ctx context.Context, enrollment localstore.Enrollment, tlsCfg tlsconfig.ClientConfig) {
	backoff := time.Second
	for ctx.Err() == nil {
		identity := runtimeIdentity{TenantID: enrollment.TenantID, AgentID: enrollment.AgentID, HostID: r.runner.currentIdentity().HostID}
		if err := r.runControlChannel(ctx, enrollment.GatewayAddress, "", tlsCfg, identity); err != nil && ctx.Err() == nil && r.runner.Out != nil {
			fmt.Fprintf(r.runner.Out, "agent managed control channel disconnected: %v\n", err)
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (r *TransportRuntime) handleControlFrame(ctx context.Context, session *ControlChannel, identity runtimeIdentity, frame *controlplanev1.ControlFrame) error {
	runner := r.runner
	switch frame.GetType() {
	case "ack":
		if frame.GetAck().GetStatus() == "rejected" {
			return fmt.Errorf("control channel request rejected: %s", frame.GetAck().GetMessage())
		}
		return nil
	case "policy_update":
		requestContext := frame.GetContext()
		if requestContext == nil {
			requestContext = &controlplanev1.RequestContext{RequestId: frame.GetRequestId()}
		}
		controller := newPolicyController(runner, r.sensor, r.batcher)
		ack := controlAck(controller.ApplyPolicy(ctx, policyCommand(&controlplanev1.ApplyPolicyRequest{
			Context: requestContext, PolicyType: "endpoint", PolicyJson: frame.GetPolicyUpdate().GetRawJson(),
		}, agentcontrol.PolicySourceManaged)))
		ack = bindControlAckToSession(ack, identity)
		if err := session.SendControlAck(ctx, ack); err != nil {
			return err
		}
		if runner.Out != nil {
			fmt.Fprintf(runner.Out, "agent control policy update ack: request=%s status=%s message=%q\n", ack.GetRequestId(), ack.GetStatus(), ack.GetMessage())
		}
		return nil
	case "content_update":
		req := contentUpdateFromControlFrame(frame)
		controller := newContentController(runner)
		ack := controlAck(controller.ApplyContent(ctx, contentCommand(req, agentcontrol.PolicySourceManaged)))
		ack = bindControlAckToSession(ack, identity)
		if err := session.SendControlAck(ctx, ack); err != nil {
			return err
		}
		if runner.Out != nil {
			fmt.Fprintf(runner.Out, "agent control content update ack: request=%s status=%s message=%q\n", ack.GetRequestId(), ack.GetStatus(), ack.GetMessage())
		}
		return nil
	case "resume":
		cursor := frame.GetResume().GetResumeCursor()
		if runner.Out != nil {
			fmt.Fprintf(runner.Out, "agent control resume cursor ignored by telemetry data plane: %s\n", cursor)
		}
		return nil
	case "response_command":
		cmd, err := responseCommandFromControl(frame.GetResponseCommand())
		if err != nil {
			return err
		}
		ack := runner.executeResponse(ctx, cmd)
		if err := session.SendResponseAck(ctx, ack); err != nil {
			return err
		}
		if runner.Out != nil {
			fmt.Fprintf(runner.Out, "agent control response ack: response=%s action=%s observe_only=%t unsupported=%t executed=%t\n", ack.ResponseID, cmd.Action, ack.ObserveOnly, ack.Unsupported, ack.Executed)
		}
		return nil
	case "evidence_pullback":
		req, err := evidencePullbackFromControl(frame.GetEvidencePullback())
		if err != nil {
			return err
		}
		result := runner.collectEvidencePullback(req)
		if err := session.SendEvidenceResult(ctx, result); err != nil {
			return err
		}
		if runner.Out != nil {
			fmt.Fprintf(runner.Out, "agent control evidence pullback result: request=%s ok=%t message=%q\n", result.RequestID, result.OK, result.Message)
		}
		return nil
	default:
		return nil
	}
}

func contentUpdateFromControlFrame(frame *controlplanev1.ControlFrame) *controlplanev1.ApplyContentRequest {
	if frame == nil {
		return &controlplanev1.ApplyContentRequest{}
	}
	req := protoCloneApplyContentRequest(frame.GetContentUpdate())
	if req.Context == nil {
		req.Context = frame.GetContext()
	}
	if req.Context == nil {
		req.Context = &controlplanev1.RequestContext{}
	}
	if req.Context.RequestId == "" {
		req.Context.RequestId = frame.GetRequestId()
	}
	if req.Context.TenantId == "" {
		req.Context.TenantId = frame.GetContext().GetTenantId()
	}
	if req.Context.AgentId == "" {
		req.Context.AgentId = frame.GetContext().GetAgentId()
	}
	if req.Context.Scope == nil {
		req.Context.Scope = frame.GetContext().GetScope()
	}
	return req
}

func protoCloneApplyContentRequest(in *controlplanev1.ApplyContentRequest) *controlplanev1.ApplyContentRequest {
	if in == nil {
		return &controlplanev1.ApplyContentRequest{}
	}
	return proto.Clone(in).(*controlplanev1.ApplyContentRequest)
}
