package tetragon

import (
	"context"
	"os"
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
