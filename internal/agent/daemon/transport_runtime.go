package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
)

func (r *TransportRuntime) runControlFlow(ctx context.Context) {
	runner := r.runner
	backoff := runner.Config.Upload.RetryInitial
	if backoff <= 0 {
		backoff = time.Second
	}
	maxBackoff := runner.Config.Upload.RetryMax
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
	connectCtx, cancel := context.WithTimeout(ctx, runner.Config.Upload.RequestTimeout)
	defer cancel()
	session := NewControlChannel(runner.Config.Manager.Address, runner.Config.Agent.Token, runner.managerTLS())
	if err := session.Open(connectCtx); err != nil {
		return err
	}
	defer session.Close()
	frames, err := session.Hello(connectCtx, runner.Config.Agent.TenantID, runner.Config.Agent.ID, r.scopeType, r.scopeSelector)
	if err != nil {
		return err
	}
	for _, frame := range frames {
		if err := r.handleControlFrame(ctx, session, frame); err != nil {
			return err
		}
	}
	health, err := runner.collectHealth(ctx, r.sensor, r.spool.Queue(), r.worker, r.startedAt)
	if err == nil {
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
			if err := r.handleControlFrame(ctx, session, frame); err != nil {
				return err
			}
		case <-ticker.C:
			health, err := runner.collectHealth(ctx, r.sensor, r.spool.Queue(), r.worker, r.startedAt)
			if err != nil {
				return err
			}
			if err := session.SendHealth(ctx, health); err != nil {
				return err
			}
			if err := session.SendCapability(ctx, health); err != nil {
				return err
			}
		}
	}
}

func (r *TransportRuntime) handleControlFrame(ctx context.Context, session *ControlChannel, frame *controlplanev1.ControlFrame) error {
	runner := r.runner
	switch frame.GetType() {
	case "ack":
		if frame.GetAck().GetStatus() == "rejected" {
			return fmt.Errorf("control channel request rejected: %s", frame.GetAck().GetMessage())
		}
		return nil
	case "policy_update":
		policy, err := policyFromControlFrame(frame.GetPolicyUpdate())
		if err != nil {
			return err
		}
		if !samePolicyRuntime(runner.activePolicy(), policy) {
			runner.applyRuntimePolicy(policy)
			if runner.Out != nil {
				fmt.Fprintf(runner.Out, "agent control policy update: policy=%s version=%d mode=%s\n", policy.PolicyID, policy.Version, policy.Mode)
			}
		}
		return nil
	case "resume":
		cursor := frame.GetResume().GetResumeCursor()
		if err := r.spool.AckThrough(cursor); err != nil {
			return err
		}
		if runner.Out != nil {
			fmt.Fprintf(runner.Out, "agent control resume cursor: %s\n", cursor)
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
