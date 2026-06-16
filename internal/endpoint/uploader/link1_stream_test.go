package uploader

import (
	"net"
	"testing"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	transportlink1 "github.com/sysarmor/sysarmor-next-project/internal/transport/link1"
	"google.golang.org/grpc"
)

func TestStreamUploaderUploadsThroughLink1Stream(t *testing.T) {
	st := &store.Store{}
	linkSrv := transportlink1.NewServer(st)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterLink1Server(grpcServer, transportlink1.NewGRPCServer(linkSrv))
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
	sessions := st.ListLink1Sessions("default", "stream-uploader-agent")
	if len(sessions) != 1 || sessions[0].LastAckCursor != "stream-uploader-batch" || sessions[0].Transport != "stream" {
		t.Fatalf("sessions = %+v", sessions)
	}
}
