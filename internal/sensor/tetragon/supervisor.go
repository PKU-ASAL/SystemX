package tetragon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
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

type ProcessSupervisor struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	done         chan struct{}
	running      bool
	restartCount uint64
	lastExit     string
	lastError    string
}

func (s *ProcessSupervisor) Start(ctx context.Context, spec ProcessSpec) error {
	if strings.TrimSpace(spec.Path) == "" {
		return fmt.Errorf("process path is required")
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("%s process is already running", firstNonEmpty(spec.Name, spec.Path))
	}
	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, spec.Path, spec.Args...)
	cmd.Env = append(os.Environ(), spec.Env...)
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
		return err
	}

	go s.wait(procCtx, cmd, done)
	return nil
}

func (s *ProcessSupervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()
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
