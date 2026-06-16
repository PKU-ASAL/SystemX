package link1

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	agenthealth "github.com/sysarmor/sysarmor-next-project/internal/agent/health"
	link1model "github.com/sysarmor/sysarmor-next-project/internal/link1"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestGRPCUpload(t *testing.T) {
	st := &store.Store{}
	server := NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterLink1Server(grpcServer, NewGRPCServer(server))
	lis := bufconn.Listen(1024 * 1024)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ack, err := analyticsv1.NewLink1Client(conn).Upload(ctx, &analyticsv1.UploadBatch{
		BatchId: "00000000000000000007",
		Agent:   &analyticsv1.AgentHello{AgentId: "grpc-agent", HostId: "grpc-host", TenantId: "default"},
		Signals: []*signalv1.Signal{
			endpointSignal("web_runtime_spawns_shell", "lin-a", false, processEntity("p-web")),
			endpointSignal("payload_dropped", "lin-a", false, fileEntity("/dev/shm/x.sh")),
			endpointSignal("reverse_shell_pattern", "lin-a", true, processEntity("p-bash"), socketEntity("10.66.0.99:443")),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ack.GetOk() || ack.GetAcceptedSignals() != 3 || ack.GetBatchId() != "00000000000000000007" {
		t.Fatalf("ack = %#v, want ok with 3 accepted signals and batch id", ack)
	}
	if got := st.ListIncidents("apt-fileless-c2"); len(got) != 1 {
		t.Fatalf("incidents = %d, want 1", len(got))
	}
}

func TestGRPCStreamExchangesDownlinkAndUplinkFrames(t *testing.T) {
	st := &store.Store{}
	st.CreateResponse(responsemodel.Command{
		ResponseID: "resp-stream",
		TenantID:   "default",
		AgentID:    "stream-agent",
		Action:     "collect",
		Target:     "process:p1",
	})
	st.CreateEvidencePullback(link1model.EvidencePullbackRequest{
		RequestID: "evpb-stream",
		TenantID:  "default",
		AgentID:   "stream-agent",
		Target:    "process:p1",
	})
	server := NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterLink1Server(grpcServer, NewGRPCServer(server))
	lis := bufconn.Listen(1024 * 1024)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	stream, err := analyticsv1.NewLink1Client(conn).Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&analyticsv1.StreamFrame{
		Type:        "hello",
		PayloadJson: []byte(`{"tenant_id":"default","agent_id":"stream-agent"}`),
	}); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	downlink, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv downlink: %v", err)
	}
	if downlink.GetType() != "downlink" {
		t.Fatalf("downlink type = %q", downlink.GetType())
	}
	if sessions := st.ListLink1Sessions("default", "stream-agent"); len(sessions) != 1 || sessions[0].Status != "open" || sessions[0].Transport != "stream" || !sessions[0].ClosedAt.IsZero() {
		t.Fatalf("session after hello = %+v", sessions)
	}
	body := string(downlink.GetPayloadJson())
	for _, want := range []string{`"type":"resume"`, `"type":"policy_update"`, `"type":"response_command"`, `"response_id":"resp-stream"`, `"type":"evidence_pullback"`, `"request_id":"evpb-stream"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("downlink missing %s: %s", want, body)
		}
	}

	batch := &analyticsv1.UploadBatch{
		BatchId: "stream-batch-1",
		Agent:   &analyticsv1.AgentHello{AgentId: "stream-agent", HostId: "stream-host", TenantId: "default"},
	}
	payload, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&analyticsv1.StreamFrame{Type: UplinkUpload, PayloadJson: payload}); err != nil {
		t.Fatalf("send upload: %v", err)
	}
	ack, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv upload ack: %v", err)
	}
	if ack.GetType() != UplinkUpload {
		t.Fatalf("ack type = %q", ack.GetType())
	}
	var result UplinkFrameResult
	if err := json.Unmarshal(ack.GetPayloadJson(), &result); err != nil {
		t.Fatalf("decode result: %v body=%s", err, string(ack.GetPayloadJson()))
	}
	if !result.OK || result.BatchID != "stream-batch-1" {
		t.Fatalf("stream result = %+v", result)
	}
	if sessions := st.ListLink1Sessions("default", "stream-agent"); len(sessions) != 1 || sessions[0].LastAckCursor != "stream-batch-1" || sessions[0].Transport != "stream" {
		t.Fatalf("sessions = %+v", sessions)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close stream: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		sessions := st.ListLink1Sessions("default", "stream-agent")
		if len(sessions) == 1 && sessions[0].Status == "closed" && !sessions[0].ClosedAt.IsZero() && sessions[0].LastAckCursor == "stream-batch-1" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session did not close: %+v", sessions)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestGRPCStreamAcceptsHealthHeartbeatFrame(t *testing.T) {
	st := &store.Store{}
	server := NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterLink1Server(grpcServer, NewGRPCServer(server))
	lis := bufconn.Listen(1024 * 1024)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	stream, err := analyticsv1.NewLink1Client(conn).Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	health := agenthealth.AgentHealth{
		AgentID:    "stream-health-agent",
		HostID:     "stream-health-host",
		TenantID:   "default",
		Status:     "ok",
		ObservedAt: time.Now().UTC(),
		Scope:      agenthealth.RuntimeScope{Type: "container", Selector: "checkout-api"},
		Sensor:     agenthealth.SensorHealth{Backend: "fake", Running: true, EventsSeen: 7},
	}
	payload, err := json.Marshal(health)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&analyticsv1.StreamFrame{Type: UplinkHealth, PayloadJson: payload}); err != nil {
		t.Fatalf("send health frame: %v", err)
	}
	ack, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv health ack: %v", err)
	}
	if ack.GetType() != UplinkHealth {
		t.Fatalf("ack type = %q", ack.GetType())
	}
	var result UplinkFrameResult
	if err := json.Unmarshal(ack.GetPayloadJson(), &result); err != nil {
		t.Fatalf("decode result: %v body=%s", err, string(ack.GetPayloadJson()))
	}
	if !result.OK || result.Message != "accepted" {
		t.Fatalf("stream health result = %+v", result)
	}
	got, ok := st.GetAgentHealth("default", "stream-health-agent")
	if !ok || got.Status != "ok" || got.Sensor.EventsSeen != 7 || got.Scope.Selector != "checkout-api" {
		t.Fatalf("agent health = %+v, ok=%v", got, ok)
	}
}

func TestGRPCAuthRequiresDevToken(t *testing.T) {
	st := &store.Store{}
	server := NewServerWithAuth(st, "dev-token")
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterLink1Server(grpcServer, NewGRPCServer(server))
	lis := bufconn.Listen(1024 * 1024)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = analyticsv1.NewLink1Client(conn).Upload(ctx, &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "grpc-agent", HostId: "grpc-host", TenantId: "default"},
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("Upload() error = %v, want unauthenticated", err)
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", "dev-token")
	ack, err := analyticsv1.NewLink1Client(conn).Upload(ctx, &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "grpc-agent", HostId: "grpc-host", TenantId: "default"},
	})
	if err != nil {
		t.Fatalf("Upload() with token error = %v", err)
	}
	if !ack.GetOk() {
		t.Fatalf("ack = %#v", ack)
	}
}

func TestGRPCUploadRequiresAgentIdentity(t *testing.T) {
	st := &store.Store{}
	server := NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterLink1Server(grpcServer, NewGRPCServer(server))
	lis := bufconn.Listen(1024 * 1024)
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, err = analyticsv1.NewLink1Client(conn).Upload(ctx, &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "grpc-agent", HostId: "grpc-host"},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("Upload() error = %v, want invalid argument", err)
	}
}
