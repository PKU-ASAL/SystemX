package gateway_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	controlmodel "github.com/sysarmor/sysarmor-next-project/internal/controlmodel"
	"github.com/sysarmor/sysarmor-next-project/internal/gateway"
	policymodel "github.com/sysarmor/sysarmor-next-project/internal/policy"
	responsemodel "github.com/sysarmor/sysarmor-next-project/internal/response"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	ingestworker "github.com/sysarmor/sysarmor-next-project/internal/workers/ingest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestDataPlaneAppendBatch(t *testing.T) {
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st, LocalProcessor: ingestworker.NewProcessor(st, nil)})
	grpcServer := grpc.NewServer()
	dataplanev1.RegisterAgentDataPlaneServiceServer(grpcServer, gateway.NewDataServer(server))
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

	ack, err := dataplanev1.NewAgentDataPlaneServiceClient(conn).AppendBatch(ctx, grpcDataBatch("00000000000000000007", "grpc-agent", "grpc-host", []*signalv1.Signal{
		endpointSignal("web_runtime_spawns_shell", "lin-a", false, processEntity("p-web")),
		endpointSignal("payload_dropped", "lin-a", false, fileEntity("/dev/shm/x.sh")),
		endpointSignal("reverse_shell_pattern", "lin-a", true, processEntity("p-bash"), socketEntity("10.66.0.99:443")),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !ack.GetAccepted() || ack.GetBatchId() != "00000000000000000007" {
		t.Fatalf("ack = %#v, want accepted with batch id", ack)
	}
	if ack.GetPartial() {
		t.Fatalf("ack partial = true, want false until partial append is explicitly supported")
	}
	if got := st.ListIncidents(store.LabelSelector{"scenario": "apt-fileless-c2"}); len(got) != 1 {
		t.Fatalf("incidents = %d, want 1", len(got))
	}
}

func TestDataPlaneAppendBatchDuplicateReturnsCommittedAck(t *testing.T) {
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st, LocalProcessor: ingestworker.NewProcessor(st, nil)})
	grpcServer := grpc.NewServer()
	dataplanev1.RegisterAgentDataPlaneServiceServer(grpcServer, gateway.NewDataServer(server))
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

	client := dataplanev1.NewAgentDataPlaneServiceClient(conn)
	batch := grpcDataBatch("duplicate-batch", "grpc-agent", "grpc-host", nil)
	if ack, err := client.AppendBatch(ctx, batch); err != nil || !ack.GetAccepted() || ack.GetStatus() != dataplanev1.DataAck_STATUS_ACCEPTED {
		t.Fatalf("first AppendBatch ack=%+v err=%v, want accepted", ack, err)
	}
	ack, err := client.AppendBatch(ctx, batch)
	if err != nil {
		t.Fatalf("duplicate AppendBatch error = %v", err)
	}
	if !ack.GetAccepted() || ack.GetStatus() != dataplanev1.DataAck_STATUS_DUPLICATE || ack.GetReasonCode() != "duplicate" || ack.GetCommittedCursor() != "duplicate-batch" || ack.GetRetryable() {
		t.Fatalf("duplicate ack = %+v, want committed duplicate", ack)
	}
}

func TestAgentDataPlaneServiceMTLSBindsBatchIdentity(t *testing.T) {
	certs := writeTestMTLSFiles(t, "default", "grpc-agent")
	serverOpt, err := tlsconfig.ServerOption(certs.serverCert, certs.serverKey, certs.ca, true)
	if err != nil {
		t.Fatal(err)
	}
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st, LocalProcessor: ingestworker.NewProcessor(st, nil)})
	grpcServer := grpc.NewServer(serverOpt)
	dataplanev1.RegisterAgentDataPlaneServiceServer(grpcServer, gateway.NewDataServer(server))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	creds, err := tlsconfig.ClientCredentials(tlsconfig.ClientConfig{
		CAFile:     certs.ca,
		CertFile:   certs.clientCert,
		KeyFile:    certs.clientKey,
		ServerName: "localhost",
	})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.DialContext(context.Background(), lis.Addr().String(), grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := dataplanev1.NewAgentDataPlaneServiceClient(conn)
	if _, err := client.AppendBatch(context.Background(), grpcDataBatch("mtls-ok", "grpc-agent", "grpc-host", nil)); err != nil {
		t.Fatalf("AppendBatch() matching mTLS identity error = %v", err)
	}
	_, err = client.AppendBatch(context.Background(), grpcDataBatch("mtls-denied", "other-agent", "grpc-host", nil))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("AppendBatch() mismatched mTLS identity error = %v, want permission denied", err)
	}
}

func TestControlPlaneConnectMTLSBindsFrameIdentity(t *testing.T) {
	certs := writeTestMTLSFiles(t, "default", "control-agent")
	serverOpt, err := tlsconfig.ServerOption(certs.serverCert, certs.serverKey, certs.ca, true)
	if err != nil {
		t.Fatal(err)
	}
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st})
	grpcServer := grpc.NewServer(serverOpt)
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, gateway.NewControlServer(server))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	creds, err := tlsconfig.ClientCredentials(tlsconfig.ClientConfig{
		CAFile:     certs.ca,
		CertFile:   certs.clientCert,
		KeyFile:    certs.clientKey,
		ServerName: "localhost",
	})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.DialContext(context.Background(), lis.Addr().String(), grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	stream, err := controlplanev1.NewAgentControlPlaneServiceClient(conn).Connect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "hello",
		RequestId:       "mtls-control-ok",
		ContractVersion: 1,
		Sequence:        1,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "control-agent", Scope: &controlplanev1.Scope{Type: "host"}},
	}); err != nil {
		t.Fatalf("send matching hello: %v", err)
	}
	frame, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv matching hello response: %v", err)
	}
	if frame.GetType() != "policy_update" {
		t.Fatalf("matching hello frame = %+v, want policy_update", frame)
	}
	if frame.GetContractVersion() != 1 {
		t.Fatalf("policy_update contract_version = %d, want 1", frame.GetContractVersion())
	}
	frame, err = stream.Recv()
	if err != nil {
		t.Fatalf("recv matching hello resume: %v", err)
	}
	if frame.GetType() != "resume" {
		t.Fatalf("matching hello frame = %+v, want resume", frame)
	}
	agents := st.ListAgents()
	if len(agents) != 1 || agents[0].AuthType != "mtls" || agents[0].CertIdentity == "" {
		t.Fatalf("registered agents = %+v, want mTLS identity binding", agents)
	}

	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "hello",
		RequestId:       "mtls-control-denied",
		ContractVersion: 1,
		Sequence:        2,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "other-agent", Scope: &controlplanev1.Scope{Type: "host"}},
	}); err != nil {
		t.Fatalf("send mismatched hello: %v", err)
	}
	frame, err = stream.Recv()
	if err != nil {
		t.Fatalf("recv mismatched hello response: %v", err)
	}
	if frame.GetType() != "ack" || frame.GetAck().GetStatus() != "rejected" {
		t.Fatalf("mismatched hello frame = %+v, want rejected ack", frame)
	}
	if frame.GetError().GetCode() != codes.PermissionDenied.String() || frame.GetError().GetRetryable() {
		t.Fatalf("mismatched hello error = %+v, want non-retryable permission denied", frame.GetError())
	}
}

func TestControlPlaneConnectAcceptsHealthReport(t *testing.T) {
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st})
	grpcServer := grpc.NewServer()
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, gateway.NewControlServer(server))
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

	stream, err := controlplanev1.NewAgentControlPlaneServiceClient(conn).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "health_report",
		RequestId:       "health-test",
		ContractVersion: 1,
		Sequence:        1,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "control-agent", Scope: &controlplanev1.Scope{Type: "host"}},
		Health: &controlplanev1.HealthResponse{
			AgentId:    "control-agent",
			HostId:     "control-host",
			TenantId:   "default",
			Status:     "ok",
			Scope:      &controlplanev1.Scope{Type: "host"},
			ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Capability: &controlplanev1.SensorCapability{Backend: "fake", Version: "dev", SupportsHealth: true},
			Sensor:     &controlplanev1.SensorHealth{Backend: "fake", Running: true, EventsSeen: 9},
		},
	}); err != nil {
		t.Fatalf("send health report: %v", err)
	}
	ack, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv ack: %v", err)
	}
	if ack.GetType() != "ack" || ack.GetAck().GetStatus() != "accepted" || ack.GetAck().GetRequestId() != "health-test" {
		t.Fatalf("ack = %+v", ack)
	}
	got, ok := st.GetAgentHealth("default", "control-agent")
	if !ok || got.Status != "ok" || got.Sensor.EventsSeen != 9 || got.Capability.Version != "dev" {
		t.Fatalf("agent health = %+v ok=%t", got, ok)
	}
}

func TestControlPlaneConnectAcceptsAgentAck(t *testing.T) {
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st})
	grpcServer := grpc.NewServer()
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, gateway.NewControlServer(server))
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

	stream, err := controlplanev1.NewAgentControlPlaneServiceClient(conn).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "ack",
		RequestId:       "content-update-ack",
		ContractVersion: 1,
		Sequence:        1,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "ack-agent", Scope: &controlplanev1.Scope{Type: "host"}},
		Ack: &controlplanev1.ControlAck{
			RequestId: "content-update-1",
			TenantId:  "default",
			AgentId:   "ack-agent",
			Status:    "applied",
			Message:   "content applied",
		},
	}); err != nil {
		t.Fatalf("send ack: %v", err)
	}
	reply, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv ack reply: %v", err)
	}
	if reply.GetType() != "ack" || reply.GetAck().GetStatus() != "accepted" || reply.GetAck().GetMessage() != "ack accepted" {
		t.Fatalf("ack reply = %+v", reply)
	}
}

func TestControlPlaneConnectSequenceRejectsReplayAndGap(t *testing.T) {
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st})
	grpcServer := grpc.NewServer()
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, gateway.NewControlServer(server))
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

	stream, err := controlplanev1.NewAgentControlPlaneServiceClient(conn).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "hello",
		RequestId:       "seq-ok",
		ContractVersion: 1,
		Sequence:        1,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "seq-agent", Scope: &controlplanev1.Scope{Type: "host"}},
	}); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	policy, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv policy: %v", err)
	}
	resume, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv resume: %v", err)
	}
	if policy.GetSequence() != 1 || resume.GetSequence() != 2 || policy.GetContractVersion() != 1 || resume.GetContractVersion() != 1 {
		t.Fatalf("downlink sequence policy=%d/%d resume=%d/%d", policy.GetContractVersion(), policy.GetSequence(), resume.GetContractVersion(), resume.GetSequence())
	}

	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "health_report",
		RequestId:       "seq-idempotent",
		ContractVersion: 1,
		Sequence:        2,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "seq-agent", Scope: &controlplanev1.Scope{Type: "host"}},
		Health:          &controlplanev1.HealthResponse{AgentId: "seq-agent", TenantId: "default", Status: "ok"},
	}); err != nil {
		t.Fatalf("send idempotent original: %v", err)
	}
	firstAck, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv idempotent original ack: %v", err)
	}
	if firstAck.GetType() != "ack" || firstAck.GetSequence() != 3 || firstAck.GetAck().GetStatus() != "accepted" {
		t.Fatalf("first idempotent ack = %+v", firstAck)
	}

	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "health_report",
		RequestId:       "seq-idempotent",
		ContractVersion: 1,
		Sequence:        3,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "seq-agent", Scope: &controlplanev1.Scope{Type: "host"}},
		Health:          &controlplanev1.HealthResponse{AgentId: "seq-agent", TenantId: "default", Status: "degraded"},
	}); err != nil {
		t.Fatalf("send idempotent retry: %v", err)
	}
	retryAck, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv idempotent retry ack: %v", err)
	}
	if retryAck.GetType() != "ack" || retryAck.GetSequence() != 4 || retryAck.GetAck().GetStatus() != "accepted" || retryAck.GetAck().GetMessage() != firstAck.GetAck().GetMessage() {
		t.Fatalf("retry idempotent ack = %+v", retryAck)
	}
	got, ok := st.GetAgentHealth("default", "seq-agent")
	if !ok || got.Status != "ok" {
		t.Fatalf("agent health after idempotent retry = %+v ok=%t, want original status ok", got, ok)
	}

	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "health_report",
		RequestId:       "seq-replay",
		ContractVersion: 1,
		Sequence:        3,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "seq-agent", Scope: &controlplanev1.Scope{Type: "host"}},
		Health:          &controlplanev1.HealthResponse{AgentId: "seq-agent", TenantId: "default", Status: "ok"},
	}); err != nil {
		t.Fatalf("send replay: %v", err)
	}
	replay, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv replay ack: %v", err)
	}
	if replay.GetType() != "ack" || replay.GetSequence() != 5 || replay.GetAck().GetStatus() != "rejected" || replay.GetError().GetCode() != codes.AlreadyExists.String() || replay.GetError().GetRetryable() {
		t.Fatalf("replay ack = %+v", replay)
	}

	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "health_report",
		RequestId:       "seq-gap",
		ContractVersion: 1,
		Sequence:        5,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "seq-agent", Scope: &controlplanev1.Scope{Type: "host"}},
		Health:          &controlplanev1.HealthResponse{AgentId: "seq-agent", TenantId: "default", Status: "ok"},
	}); err != nil {
		t.Fatalf("send gap: %v", err)
	}
	gap, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv gap ack: %v", err)
	}
	if gap.GetType() != "ack" || gap.GetSequence() != 6 || gap.GetAck().GetStatus() != "rejected" || gap.GetError().GetCode() != codes.FailedPrecondition.String() || gap.GetError().GetRetryable() {
		t.Fatalf("gap ack = %+v", gap)
	}
}

func TestControlPlaneConnectReconnectReturnsResumeAndPendingCommands(t *testing.T) {
	st := &store.Store{}
	st.RecordDataBatchAppend(store.AgentIdentity{TenantID: "default", AgentID: "reconnect-agent"}, "batch-before-reconnect", "grpc", time.Unix(10, 0).UTC())
	st.CreateResponse(responsemodel.Command{
		ResponseID: "resp-reconnect",
		TenantID:   "default",
		AgentID:    "reconnect-agent",
		Action:     "collect",
		Target:     "process:p1",
	})
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st})
	grpcServer := grpc.NewServer()
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, gateway.NewControlServer(server))
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

	stream, err := controlplanev1.NewAgentControlPlaneServiceClient(conn).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "hello",
		RequestId:       "hello-reconnect",
		ContractVersion: 1,
		Sequence:        1,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "reconnect-agent", Scope: &controlplanev1.Scope{Type: "host"}},
	}); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	policy, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv policy: %v", err)
	}
	resume, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv resume: %v", err)
	}
	cmd, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv pending command: %v", err)
	}
	if policy.GetType() != "policy_update" || policy.GetSequence() != 1 {
		t.Fatalf("policy frame = %+v", policy)
	}
	if resume.GetType() != "resume" || resume.GetSequence() != 2 || resume.GetResume().GetResumeCursor() != "batch-before-reconnect" {
		t.Fatalf("resume frame = %+v", resume)
	}
	if cmd.GetType() != "response_command" || cmd.GetSequence() != 3 || cmd.GetResponseCommand().GetResponseId() != "resp-reconnect" {
		t.Fatalf("pending command frame = %+v", cmd)
	}
}

func TestControlPlaneConnectSendsPendingControlCommandAndPersistsAck(t *testing.T) {
	st := &store.Store{}
	st.CreateControlCommand(controlmodel.ControlCommand{
		CommandID:   "ctrl-content-stream",
		TenantID:    "default",
		AgentID:     "control-command-agent",
		Type:        controlmodel.ControlCommandTypeContentUpdate,
		PayloadJSON: []byte(`{"api_version":"sysarmor.content/v1","kind":"iocpack","metadata":{"id":"ioc:test","version":"v1"},"spec":{"value_type":"ip","values":["10.0.0.1"]}}`),
	})
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st})
	grpcServer := grpc.NewServer()
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, gateway.NewControlServer(server))
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

	stream, err := controlplanev1.NewAgentControlPlaneServiceClient(conn).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "hello",
		RequestId:       "hello-control-command",
		ContractVersion: 1,
		Sequence:        1,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "control-command-agent", Scope: &controlplanev1.Scope{Type: "host"}},
	}); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	for _, want := range []string{"policy_update", "resume"} {
		frame, err := stream.Recv()
		if err != nil {
			t.Fatalf("recv %s: %v", want, err)
		}
		if frame.GetType() != want {
			t.Fatalf("frame = %+v, want %s", frame, want)
		}
	}
	cmdFrame, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv control command: %v", err)
	}
	if cmdFrame.GetType() != "content_update" || cmdFrame.GetRequestId() != "ctrl-content-stream" || cmdFrame.GetContentUpdate().GetContentJson() == "" {
		t.Fatalf("control command frame = %+v", cmdFrame)
	}
	if got := st.ListControlCommands("default", "control-command-agent", ""); len(got) != 1 || got[0].Status != controlmodel.ControlCommandStatusSent {
		t.Fatalf("commands after send = %+v", got)
	}
	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "ack",
		RequestId:       "ctrl-content-stream",
		ContractVersion: 1,
		Sequence:        2,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "control-command-agent", RequestId: "ctrl-content-stream"},
		Ack: &controlplanev1.ControlAck{
			RequestId: "ctrl-content-stream",
			TenantId:  "default",
			AgentId:   "control-command-agent",
			Status:    "applied",
			Message:   "content applied",
			PolicyId:  "ioc:test",
		},
	}); err != nil {
		t.Fatalf("send command ack: %v", err)
	}
	reply, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv command ack reply: %v", err)
	}
	if reply.GetType() != "ack" || reply.GetAck().GetStatus() != "accepted" {
		t.Fatalf("ack reply = %+v", reply)
	}
	got := st.ListControlCommands("default", "control-command-agent", "")
	if len(got) != 1 || got[0].Status != controlmodel.ControlCommandStatusApplied || got[0].AckMessage != "content applied" || got[0].AckPolicyID != "ioc:test" {
		t.Fatalf("commands after ack = %+v", got)
	}
}

func TestControlPlaneConnectRequiresRequestID(t *testing.T) {
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st})
	grpcServer := grpc.NewServer()
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, gateway.NewControlServer(server))
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

	stream, err := controlplanev1.NewAgentControlPlaneServiceClient(conn).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "health_report",
		ContractVersion: 1,
		Sequence:        1,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "request-id-agent", Scope: &controlplanev1.Scope{Type: "host"}},
		Health:          &controlplanev1.HealthResponse{AgentId: "request-id-agent", TenantId: "default", Status: "ok"},
	}); err != nil {
		t.Fatalf("send missing request_id: %v", err)
	}
	ack, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv missing request_id ack: %v", err)
	}
	if ack.GetType() != "ack" || ack.GetAck().GetStatus() != "rejected" || ack.GetError().GetCode() != codes.InvalidArgument.String() {
		t.Fatalf("ack = %+v, want InvalidArgument rejection", ack)
	}
}

type testMTLSFiles struct {
	ca         string
	serverCert string
	serverKey  string
	clientCert string
	clientKey  string
}

func writeTestMTLSFiles(t *testing.T, tenantID, agentID string) testMTLSFiles {
	t.Helper()
	dir := t.TempDir()
	caKey, caCert := newTestCA(t)
	serverCert, serverKey := newTestCert(t, caCert, caKey, "localhost", nil, []string{"localhost"})
	identityURI := &url.URL{Scheme: "spiffe", Host: "sysarmor.local", Path: "/tenant/" + tenantID + "/agent/" + agentID}
	clientCert, clientKey := newTestCert(t, caCert, caKey, tenantID+"/"+agentID, []*url.URL{identityURI}, nil)
	files := testMTLSFiles{
		ca:         filepath.Join(dir, "ca.pem"),
		serverCert: filepath.Join(dir, "server.pem"),
		serverKey:  filepath.Join(dir, "server-key.pem"),
		clientCert: filepath.Join(dir, "client.pem"),
		clientKey:  filepath.Join(dir, "client-key.pem"),
	}
	writePEM(t, files.ca, "CERTIFICATE", caCert.Raw)
	writePEM(t, files.serverCert, "CERTIFICATE", serverCert.Raw)
	writePEM(t, files.serverKey, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(serverKey))
	writePEM(t, files.clientCert, "CERTIFICATE", clientCert.Raw)
	writePEM(t, files.clientKey, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(clientKey))
	return files
}

func newTestCA(t *testing.T) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "sysarmor-test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return key, cert
}

func newTestCert(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey, cn string, uris []*url.URL, dns []string) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		URIs:         uris,
		DNSNames:     dns,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key
}

func writePEM(t *testing.T, path, typ string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: typ, Bytes: der}); err != nil {
		t.Fatal(err)
	}
}

func TestControlPlaneConnectHelloReturnsPolicyUpdate(t *testing.T) {
	st := &store.Store{}
	policy := policymodel.DefaultPolicy("default")
	policy.PolicyID = "control-policy"
	policy.Version = 9
	st.UpsertPolicy(policy)
	st.AssignPolicy(policymodel.Assignment{TenantID: "default", AgentID: "control-agent", PolicyID: "control-policy", PolicyVersion: 9})
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st})
	grpcServer := grpc.NewServer()
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, gateway.NewControlServer(server))
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

	stream, err := controlplanev1.NewAgentControlPlaneServiceClient(conn).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(&controlplanev1.ControlFrame{
		Type:            "hello",
		RequestId:       "hello-policy",
		ContractVersion: 1,
		Sequence:        1,
		Context:         &controlplanev1.RequestContext{TenantId: "default", AgentId: "control-agent", Scope: &controlplanev1.Scope{Type: "host"}},
	}); err != nil {
		t.Fatalf("send hello: %v", err)
	}
	frame, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv policy update: %v", err)
	}
	if frame.GetType() != "policy_update" || frame.GetPolicyUpdate().GetPolicyId() != "control-policy" || frame.GetPolicyUpdate().GetVersion() != 9 {
		t.Fatalf("policy frame = %+v", frame)
	}
}

func TestGRPCAuthRequiresDevToken(t *testing.T) {
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st, AgentToken: "dev-token"})
	grpcServer := grpc.NewServer()
	dataplanev1.RegisterAgentDataPlaneServiceServer(grpcServer, gateway.NewDataServer(server))
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

	_, err = dataplanev1.NewAgentDataPlaneServiceClient(conn).AppendBatch(ctx, grpcDataBatch("", "grpc-agent", "grpc-host", nil))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("AppendBatch() error = %v, want unauthenticated", err)
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", "dev-token")
	ack, err := dataplanev1.NewAgentDataPlaneServiceClient(conn).AppendBatch(ctx, grpcDataBatch("", "grpc-agent", "grpc-host", nil))
	if err != nil {
		t.Fatalf("AppendBatch() with token error = %v", err)
	}
	if !ack.GetAccepted() {
		t.Fatalf("ack = %#v", ack)
	}
}

func TestDataPlaneAppendBatchRequiresAgentIdentity(t *testing.T) {
	st := &store.Store{}
	server := gateway.NewRuntime(gateway.RuntimeOptions{Store: st})
	grpcServer := grpc.NewServer()
	dataplanev1.RegisterAgentDataPlaneServiceServer(grpcServer, gateway.NewDataServer(server))
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

	ack, err := dataplanev1.NewAgentDataPlaneServiceClient(conn).AppendBatch(ctx, &dataplanev1.DataBatch{
		Header: &dataplanev1.BatchHeader{AgentId: "grpc-agent", HostId: "grpc-host"},
	})
	if err != nil {
		t.Fatalf("AppendBatch() error = %v, want structured DataAck rejection", err)
	}
	if ack.GetAccepted() || ack.GetStatus() != dataplanev1.DataAck_STATUS_REJECTED || ack.GetReasonCode() != "invalid_data_batch" || ack.GetRetryable() || ack.GetContractVersion() != "dataplane.v1" {
		t.Fatalf("ack = %+v, want non-retryable invalid_data_batch rejection", ack)
	}
}

func TestDataAckClassifiesRetryableBackendError(t *testing.T) {
	grpcServer := grpc.NewServer()
	dataplanev1.RegisterAgentDataPlaneServiceServer(grpcServer, gateway.NewDataServer(retryableUploadBackend{err: status.Error(codes.Unavailable, "durable telemetry unavailable")}))
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

	ack, err := dataplanev1.NewAgentDataPlaneServiceClient(conn).AppendBatch(ctx, grpcDataBatch("retryable-batch", "grpc-agent", "grpc-host", nil))
	if err != nil {
		t.Fatalf("AppendBatch() error = %v, want structured retryable DataAck", err)
	}
	if ack.GetAccepted() || ack.GetStatus() != dataplanev1.DataAck_STATUS_RETRYABLE || ack.GetReasonCode() != "retryable_server_error" || !ack.GetRetryable() || ack.GetRetryAfterMs() == 0 || ack.GetBatchId() != "retryable-batch" || ack.GetContractVersion() != "dataplane.v1" {
		t.Fatalf("ack = %+v, want retryable server error", ack)
	}
}

func TestDataAckClassifiesNonRetryableBackendError(t *testing.T) {
	grpcServer := grpc.NewServer()
	dataplanev1.RegisterAgentDataPlaneServiceServer(grpcServer, gateway.NewDataServer(retryableUploadBackend{err: status.Error(codes.InvalidArgument, "schema rejected")}))
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

	ack, err := dataplanev1.NewAgentDataPlaneServiceClient(conn).AppendBatch(ctx, grpcDataBatch("server-reject-batch", "grpc-agent", "grpc-host", nil))
	if err != nil {
		t.Fatalf("AppendBatch() error = %v, want structured non-retryable DataAck", err)
	}
	if ack.GetAccepted() || ack.GetStatus() != dataplanev1.DataAck_STATUS_REJECTED || ack.GetReasonCode() != "server_error" || ack.GetRetryable() || ack.GetRetryAfterMs() != 0 || ack.GetBatchId() != "server-reject-batch" || ack.GetContractVersion() != "dataplane.v1" {
		t.Fatalf("ack = %+v, want non-retryable server error", ack)
	}
}

type retryableUploadBackend struct {
	err error
}

func (b retryableUploadBackend) AgentToken() string { return "" }

func (b retryableUploadBackend) AppendDataBatchWithTransport(*dataplanev1.DataBatch, string) (gateway.DataAppendResult, error) {
	return gateway.DataAppendResult{}, b.err
}

func (b retryableUploadBackend) BindAgentIdentity(store.AgentIdentity) error { return nil }

func (b retryableUploadBackend) Store() gateway.ControlStore { return &store.Store{} }

func (b retryableUploadBackend) ResumeCursor(string, string) gateway.ResumeCursor {
	return gateway.ResumeCursor{}
}

func (b retryableUploadBackend) TouchHotSession(store.AgentSession) {}

func grpcDataBatch(batchID, agentID, hostID string, signals []*signalv1.Signal) *dataplanev1.DataBatch {
	batch := &dataplanev1.DataBatch{
		Header: &dataplanev1.BatchHeader{BatchId: batchID, AgentId: agentID, HostId: hostID, TenantId: "default"},
	}
	for _, sig := range signals {
		batch.Signals = append(batch.Signals, &dataplanev1.SignalFrame{Signal: sig})
	}
	return batch
}

func endpointSignal(name, lineage string, terminal bool, entities ...*signalv1.EntityRef) *signalv1.Signal {
	return &signalv1.Signal{
		Id:           "sig-" + name,
		Name:         name,
		Where:        signalv1.SignalWhere_SIGNAL_WHERE_ENDPOINT,
		BaseRisk:     50,
		GlobalRarity: 1,
		LineageId:    lineage,
		Terminal:     terminal,
		Labels:       map[string]string{"scenario": "apt-fileless-c2"},
		Entities:     entities,
	}
}

func processEntity(key string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "process", Key: "process:" + key, Role: "subject"}
}

func fileEntity(path string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "file", Key: "file:" + path, Role: "object"}
}

func socketEntity(addr string) *signalv1.EntityRef {
	return &signalv1.EntityRef{Kind: "socket", Key: "socket:" + addr, Role: "object"}
}
