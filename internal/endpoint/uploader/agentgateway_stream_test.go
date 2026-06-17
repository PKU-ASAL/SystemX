package uploader

import (
	"net"
	"strings"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	eventv1 "github.com/sysarmor/sysarmor-next-project/api/proto/event/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	transportgateway "github.com/sysarmor/sysarmor-next-project/internal/agentgateway"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	ingestworker "github.com/sysarmor/sysarmor-next-project/internal/workers/ingest"
	"google.golang.org/grpc"
)

func TestStreamUploaderUploadsThroughAgentGatewayStream(t *testing.T) {
	st := &store.Store{}
	linkSrv := transportgateway.NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterAgentGatewayServer(grpcServer, transportgateway.NewGRPCServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	up := NewStreamUploaderWithTimeout(lis.Addr().String(), time.Second)
	ack, err := up.Upload(&analyticsv1.UploadBatch{
		BatchId: "stream-uploader-batch",
		Agent:   &analyticsv1.AgentHello{AgentId: "stream-uploader-agent", HostId: "stream-uploader-host", TenantId: "default"},
	})
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if !ack.GetOk() || ack.GetBatchId() != "stream-uploader-batch" {
		t.Fatalf("ack = %#v", ack)
	}
	sessions := st.ListAgentGatewaySessions("default", "stream-uploader-agent")
	if len(sessions) != 1 || sessions[0].LastAckCursor != "stream-uploader-batch" || sessions[0].Transport != "stream" {
		t.Fatalf("sessions = %+v", sessions)
	}
}

func TestStreamUploaderReconnectDuplicateBatchDoesNotAmplifyIngest(t *testing.T) {
	st := &store.Store{}
	linkSrv := transportgateway.NewServer(st).WithLocalProcessor(ingestworker.NewProcessor(st, nil))
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterAgentGatewayServer(grpcServer, transportgateway.NewGRPCServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	up := NewStreamUploaderWithTimeout(lis.Addr().String(), time.Second)
	batch := &analyticsv1.UploadBatch{
		BatchId: "stream-reconnect-batch",
		Agent:   &analyticsv1.AgentHello{AgentId: "stream-reconnect-agent", HostId: "stream-reconnect-host", TenantId: "default"},
		Events: []*eventv1.CanonicalEvent{{
			Id:       "ev-stream-reconnect",
			Scenario: "stream-reconnect",
			Behavior: "process.exec",
		}},
		Signals: []*signalv1.Signal{{
			Id:       "sig-stream-reconnect",
			Scenario: "stream-reconnect",
			Name:     "reverse_shell_pattern",
			Where:    signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		}},
	}
	first, err := up.Upload(batch)
	if err != nil {
		t.Fatalf("first Upload() error = %v", err)
	}
	if !first.GetOk() || first.GetAcceptedEvents() != 1 || first.GetAcceptedSignals() != 1 {
		t.Fatalf("first ack = %#v", first)
	}
	second, err := up.Upload(batch)
	if err != nil {
		t.Fatalf("second Upload() error = %v", err)
	}
	if !second.GetOk() || second.GetBatchId() != batch.GetBatchId() || second.GetAcceptedEvents() != 0 || second.GetAcceptedSignals() != 0 {
		t.Fatalf("second ack = %#v", second)
	}
	if got := st.ListEvents("stream-reconnect", ""); len(got) != 1 || got[0].GetId() != "ev-stream-reconnect" {
		t.Fatalf("events after reconnect duplicate = %+v", got)
	}
	if got := st.ListSignals("stream-reconnect", "endpoint", false); len(got) != 1 || got[0].GetId() != "sig-stream-reconnect" {
		t.Fatalf("signals after reconnect duplicate = %+v", got)
	}
	sessions := st.ListAgentGatewaySessions("default", "stream-reconnect-agent")
	if len(sessions) != 1 || sessions[0].LastAckCursor != batch.GetBatchId() || sessions[0].Transport != "stream" {
		t.Fatalf("sessions = %+v", sessions)
	}
}

func TestStreamUploaderReportsServerErrorFrame(t *testing.T) {
	st := &store.Store{}
	linkSrv := transportgateway.NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterAgentGatewayServer(grpcServer, transportgateway.NewGRPCServer(linkSrv))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	up := NewStreamUploaderWithTimeout(lis.Addr().String(), time.Second)
	ack, err := up.Upload(&analyticsv1.UploadBatch{BatchId: "stream-error-batch"})
	if err == nil || !strings.Contains(err.Error(), "agent identity is required") {
		t.Fatalf("Upload() error = %v, ack=%#v", err, ack)
	}
	if ack == nil || ack.GetOk() || !strings.Contains(ack.GetMessage(), "agent identity is required") {
		t.Fatalf("ack = %#v", ack)
	}
}
