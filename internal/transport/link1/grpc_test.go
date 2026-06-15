package link1

import (
	"context"
	"net"
	"testing"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	signalv1 "github.com/sysarmor/sysarmor-next-project/api/proto/signal/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
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
		Agent: &analyticsv1.AgentHello{AgentId: "grpc-agent", HostId: "grpc-host"},
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
		Agent: &analyticsv1.AgentHello{AgentId: "grpc-agent", HostId: "grpc-host"},
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("Upload() error = %v, want unauthenticated", err)
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", "dev-token")
	ack, err := analyticsv1.NewLink1Client(conn).Upload(ctx, &analyticsv1.UploadBatch{
		Agent: &analyticsv1.AgentHello{AgentId: "grpc-agent", HostId: "grpc-host"},
	})
	if err != nil {
		t.Fatalf("Upload() with token error = %v", err)
	}
	if !ack.GetOk() {
		t.Fatalf("ack = %#v", ack)
	}
}
