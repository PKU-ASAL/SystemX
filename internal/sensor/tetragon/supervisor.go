package tetragon

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type ProcessSpec struct {
	Name string
	Path string
	Args []string
	Env  []string
}

type ProcessStatus struct {
	Running      bool
	RestartCount uint64
	LastExit     string
	LastError    string
}

type RestartPolicy struct {
	MaxRestarts int
	Delay       time.Duration
}

type ProcessSupervisor struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	done         chan struct{}
	loopCancel   context.CancelFunc
	loopDone     chan struct{}
	running      bool
	restartCount uint64
	lastExit     string
	lastError    string
}

func (s *ProcessSupervisor) Start(ctx context.Context, spec ProcessSpec) error {
	_, err := s.StartWithStdout(ctx, spec)
	return err
}

func (s *ProcessSupervisor) StartWithStdout(ctx context.Context, spec ProcessSpec) (io.ReadCloser, error) {
	if strings.TrimSpace(spec.Path) == "" {
		return nil, fmt.Errorf("process path is required")
	}
	s.mu.Lock()
	if s.running || s.loopCancel != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("%s process is already running", firstNonEmpty(spec.Name, spec.Path))
	}
	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, spec.Path, spec.Args...)
	cmd.Env = append(os.Environ(), spec.Env...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		s.mu.Unlock()
		return nil, err
	}
	done := make(chan struct{})
	s.cmd = cmd
	s.cancel = cancel
	s.done = done
	s.running = true
	s.restartCount++
	s.lastExit = ""
	s.lastError = ""
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		s.mu.Lock()
		s.cmd = nil
		s.cancel = nil
		s.done = nil
		s.running = false
		s.lastError = err.Error()
		s.mu.Unlock()
		cancel()
		close(done)
		return nil, err
	}

	go s.wait(procCtx, cmd, done)
	return stdout, nil
}

func (s *ProcessSupervisor) StartRestarting(ctx context.Context, spec ProcessSpec, policy RestartPolicy) error {
	if strings.TrimSpace(spec.Path) == "" {
		return fmt.Errorf("process path is required")
	}
	if policy.MaxRestarts <= 0 {
		policy.MaxRestarts = 1
	}
	if policy.Delay <= 0 {
		policy.Delay = time.Second
	}
	s.mu.Lock()
	if s.running || s.loopCancel != nil {
		s.mu.Unlock()
		return fmt.Errorf("%s process is already running", firstNonEmpty(spec.Name, spec.Path))
	}
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.loopCancel = cancel
	s.loopDone = done
	s.mu.Unlock()
	go s.restartLoop(loopCtx, spec, policy, done)
	return nil
}

func (s *ProcessSupervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	loopCancel := s.loopCancel
	loopDone := s.loopDone
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()
	if loopCancel != nil && loopDone != nil {
		loopCancel()
		select {
		case <-loopDone:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *ProcessSupervisor) restartLoop(ctx context.Context, spec ProcessSpec, policy RestartPolicy, loopDone chan struct{}) {
	defer close(loopDone)
	defer func() {
		s.mu.Lock()
		s.loopCancel = nil
		s.loopDone = nil
		s.mu.Unlock()
	}()
	for attempt := 0; attempt < policy.MaxRestarts; attempt++ {
		err := s.runProcess(ctx, spec)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			return
		}
		if attempt == policy.MaxRestarts-1 {
			return
		}
		timer := time.NewTimer(policy.Delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *ProcessSupervisor) runProcess(ctx context.Context, spec ProcessSpec) error {
	procCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(procCtx, spec.Path, spec.Args...)
	cmd.Env = append(os.Environ(), spec.Env...)
	done := make(chan struct{})
	s.mu.Lock()
	s.cmd = cmd
	s.cancel = cancel
	s.done = done
	s.running = true
	s.restartCount++
	s.lastExit = ""
	s.lastError = ""
	s.mu.Unlock()
	if err := cmd.Start(); err != nil {
		s.mu.Lock()
		s.cmd = nil
		s.cancel = nil
		s.done = nil
		s.running = false
		s.lastError = err.Error()
		s.mu.Unlock()
		close(done)
		return err
	}
	err := cmd.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	close(done)
	s.running = false
	s.cmd = nil
	s.cancel = nil
	s.done = nil
	if procCtx.Err() != nil {
		s.lastExit = "stopped"
		return nil
	}
	if err != nil {
		s.lastExit = err.Error()
		s.lastError = err.Error()
		return err
	}
	s.lastExit = "exited"
	return nil
}

func (s *ProcessSupervisor) Status() ProcessStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ProcessStatus{
		Running:      s.running,
		RestartCount: s.restartCount,
		LastExit:     s.lastExit,
		LastError:    s.lastError,
	}
}

func (s *ProcessSupervisor) wait(ctx context.Context, cmd *exec.Cmd, done chan struct{}) {
	err := cmd.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	defer close(done)
	s.running = false
	s.cmd = nil
	s.cancel = nil
	s.done = nil
	if ctx.Err() != nil {
		s.lastExit = "stopped"
		return
	}
	if err != nil {
		s.lastExit = err.Error()
		s.lastError = err.Error()
		return
	}
	s.lastExit = "exited"
}
