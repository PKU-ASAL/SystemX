package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	controlv1 "github.com/sysarmor/sysarmor-next-project/api/proto/control/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/spool"
	"github.com/sysarmor/sysarmor-next-project/internal/agent/uploadworker"
	sensorruntime "github.com/sysarmor/sysarmor-next-project/internal/sensor/runtime"
)

func (r *Runner) runControlStreamLoop(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *uploadworker.Worker, startedAt time.Time, scopeType, scopeSelector string) {
	backoff := r.Config.Upload.RetryInitial
	if backoff <= 0 {
		backoff = time.Second
	}
	maxBackoff := r.Config.Upload.RetryMax
	if maxBackoff <= 0 {
		maxBackoff = 30 * time.Second
	}
	for {
		if err := r.runControlStreamSession(ctx, rt, queue, worker, startedAt, scopeType, scopeSelector); err != nil && ctx.Err() == nil && r.Out != nil {
			fmt.Fprintf(r.Out, "agent control stream disconnected: %v\n", err)
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

func (r *Runner) runControlStreamSession(ctx context.Context, rt sensorruntime.Runtime, queue *spool.Queue, worker *uploadworker.Worker, startedAt time.Time, scopeType, scopeSelector string) error {
	connectCtx, cancel := context.WithTimeout(ctx, r.Config.Upload.RequestTimeout)
	defer cancel()
	session := NewControlStreamSession(r.Config.Manager.Address, r.Config.Agent.Token, r.managerTLS())
	if err := session.Open(connectCtx); err != nil {
		return err
	}
	defer session.Close()
	frames, err := session.Hello(connectCtx, r.Config.Agent.TenantID, r.Config.Agent.ID, scopeType, scopeSelector)
	if err != nil {
		return err
	}
	for _, frame := range frames {
		if err := r.handleControlStreamFrame(ctx, session, frame, queue); err != nil {
			return err
		}
	}
	health, err := r.collectHealth(ctx, rt, queue, worker, startedAt)
	if err == nil {
		if err := session.SendHealth(ctx, health); err != nil {
			return err
		}
		if err := session.SendCapability(ctx, health); err != nil {
			return err
		}
	}
	recvCh := make(chan *controlv1.ControlStreamFrame, 1)
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
	interval := r.Config.Health.Interval
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
				return fmt.Errorf("control stream closed")
			}
			return err
		case frame := <-recvCh:
			if err := r.handleControlStreamFrame(ctx, session, frame, queue); err != nil {
				return err
			}
		case <-ticker.C:
			health, err := r.collectHealth(ctx, rt, queue, worker, startedAt)
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

func (r *Runner) handleControlStreamFrame(ctx context.Context, session *ControlStreamSession, frame *controlv1.ControlStreamFrame, queue *spool.Queue) error {
	switch frame.GetType() {
	case "ack":
		if frame.GetAck().GetStatus() == "rejected" {
			return fmt.Errorf("control stream request rejected: %s", frame.GetAck().GetMessage())
		}
		return nil
	case "policy_update":
		policy, err := policyFromControlFrame(frame.GetPolicyUpdate())
		if err != nil {
			return err
		}
		if !samePolicyRuntime(r.activePolicy(), policy) {
			r.applyRuntimePolicy(policy)
			if r.Out != nil {
				fmt.Fprintf(r.Out, "agent control policy update: policy=%s version=%d mode=%s\n", policy.PolicyID, policy.Version, policy.Mode)
			}
		}
		return nil
	case "resume":
		cursor := frame.GetResume().GetResumeCursor()
		if cursor == "" || queue == nil {
			return nil
		}
		if err := queue.AckThrough(cursor); err != nil {
			return err
		}
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent control resume cursor: %s\n", cursor)
		}
		return nil
	case "response_command":
		cmd, err := responseCommandFromControl(frame.GetResponseCommand())
		if err != nil {
			return err
		}
		ack := r.executeResponse(ctx, cmd)
		if err := session.SendResponseAck(ctx, ack); err != nil {
			return err
		}
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent control response ack: response=%s action=%s observe_only=%t unsupported=%t executed=%t\n", ack.ResponseID, cmd.Action, ack.ObserveOnly, ack.Unsupported, ack.Executed)
		}
		return nil
	case "evidence_pullback":
		req, err := evidencePullbackFromControl(frame.GetEvidencePullback())
		if err != nil {
			return err
		}
		result := r.collectEvidencePullback(req)
		if err := session.SendEvidenceResult(ctx, result); err != nil {
			return err
		}
		if r.Out != nil {
			fmt.Fprintf(r.Out, "agent control evidence pullback result: request=%s ok=%t message=%q\n", result.RequestID, result.OK, result.Message)
		}
		return nil
	default:
		return nil
	}
}
