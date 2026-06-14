package tetragon

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestProcessSupervisorStartsAndStopsProcess(t *testing.T) {
	sh := requireShell(t)
	supervisor := &ProcessSupervisor{}
	if err := supervisor.Start(context.Background(), ProcessSpec{Name: "sleep", Path: sh, Args: []string{"-c", "sleep 5"}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status := supervisor.Status(); !status.Running || status.RestartCount != 1 {
		t.Fatalf("status after start = %+v", status)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := supervisor.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if status := supervisor.Status(); status.Running || status.LastExit != "stopped" {
		t.Fatalf("status after stop = %+v", status)
	}
}

func TestProcessSupervisorRecordsFailedExit(t *testing.T) {
	sh := requireShell(t)
	supervisor := &ProcessSupervisor{}
	if err := supervisor.Start(context.Background(), ProcessSpec{Name: "fail", Path: sh, Args: []string{"-c", "exit 7"}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status := supervisor.Status()
		if !status.Running {
			if status.LastError == "" || !strings.Contains(status.LastExit, "exit status 7") {
				t.Fatalf("status after failed exit = %+v", status)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for process exit")
}

func TestProcessSupervisorRejectsDuplicateStart(t *testing.T) {
	sh := requireShell(t)
	supervisor := &ProcessSupervisor{}
	if err := supervisor.Start(context.Background(), ProcessSpec{Name: "sleep", Path: sh, Args: []string{"-c", "sleep 5"}}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer supervisor.Stop(context.Background())
	if err := supervisor.Start(context.Background(), ProcessSpec{Name: "sleep", Path: sh, Args: []string{"-c", "sleep 5"}}); err == nil {
		t.Fatal("second Start() error = nil")
	}
}

func TestProcessSupervisorRestartsFailedProcessUntilLimit(t *testing.T) {
	sh := requireShell(t)
	dir := t.TempDir()
	countPath := filepath.Join(dir, "count")
	supervisor := &ProcessSupervisor{}
	err := supervisor.StartRestarting(context.Background(), ProcessSpec{
		Name: "fail",
		Path: sh,
		Args: []string{"-c", "n=0; if [ -f \"$COUNT\" ]; then n=$(cat \"$COUNT\"); fi; n=$((n+1)); printf '%s' \"$n\" > \"$COUNT\"; exit 7"},
		Env:  []string{"COUNT=" + countPath},
	}, RestartPolicy{MaxRestarts: 3, Delay: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("StartRestarting() error = %v", err)
	}
	waitForStatus(t, supervisor, func(status ProcessStatus) bool {
		return !status.Running && status.RestartCount == 3 && strings.Contains(status.LastExit, "exit status 7")
	})
	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("ReadFile(count) error = %v", err)
	}
	if string(data) != "3" {
		t.Fatalf("restart count file = %q, want 3", string(data))
	}
}

func TestProcessSupervisorStopsRestartLoopAndProcess(t *testing.T) {
	sh := requireShell(t)
	supervisor := &ProcessSupervisor{}
	if err := supervisor.StartRestarting(context.Background(), ProcessSpec{Name: "sleep", Path: sh, Args: []string{"-c", "sleep 5"}}, RestartPolicy{MaxRestarts: 10, Delay: time.Millisecond}); err != nil {
		t.Fatalf("StartRestarting() error = %v", err)
	}
	waitForStatus(t, supervisor, func(status ProcessStatus) bool {
		return status.Running && status.RestartCount == 1
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := supervisor.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if status := supervisor.Status(); status.Running || status.LastExit != "stopped" {
		t.Fatalf("status after stop = %+v", status)
	}
}

func TestProcessSupervisorRejectsDuplicateRestartLoop(t *testing.T) {
	sh := requireShell(t)
	supervisor := &ProcessSupervisor{}
	if err := supervisor.StartRestarting(context.Background(), ProcessSpec{Name: "sleep", Path: sh, Args: []string{"-c", "sleep 5"}}, RestartPolicy{MaxRestarts: 10, Delay: time.Millisecond}); err != nil {
		t.Fatalf("StartRestarting() error = %v", err)
	}
	defer supervisor.Stop(context.Background())
	if err := supervisor.StartRestarting(context.Background(), ProcessSpec{Name: "sleep", Path: sh, Args: []string{"-c", "sleep 5"}}, RestartPolicy{MaxRestarts: 10, Delay: time.Millisecond}); err == nil {
		t.Fatal("second StartRestarting() error = nil")
	}
	if err := supervisor.Start(context.Background(), ProcessSpec{Name: "sleep", Path: sh, Args: []string{"-c", "sleep 5"}}); err == nil {
		t.Fatal("Start() during restart loop error = nil")
	}
}

func waitForStatus(t *testing.T, supervisor *ProcessSupervisor, done func(ProcessStatus) bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var last ProcessStatus
	for time.Now().Before(deadline) {
		last = supervisor.Status()
		if done(last) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for status, last = %+v", last)
}

func requireShell(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell process supervisor tests require /bin/sh")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	return "/bin/sh"
}
