package link1

import (
	"context"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type grpcServer struct {
	analyticsv1.UnimplementedLink1Server
	srv *Server
}

func NewGRPCServer(srv *Server) analyticsv1.Link1Server {
	return &grpcServer{srv: srv}
}

func (s *grpcServer) Upload(ctx context.Context, batch *analyticsv1.UploadBatch) (*analyticsv1.UploadAck, error) {
	if !s.authorized(ctx) {
		return nil, status.Error(codes.Unauthenticated, "unauthorized")
	}
	result, err := s.srv.AcceptUpload(batch)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "accept upload: %v", err)
	}
	return &analyticsv1.UploadAck{
		Ok:              true,
		Message:         "accepted",
		AcceptedEvents:  uint64(result.AcceptedEvents),
		AcceptedSignals: uint64(result.AcceptedSignals),
		BatchId:         batch.GetBatchId(),
	}, nil
}

func (s *grpcServer) authorized(ctx context.Context) bool {
	if s.srv.authToken == "" {
		return true
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	for _, value := range md.Get("x-sysarmor-agent-token") {
		if value == s.srv.authToken {
			return true
		}
	}
	for _, value := range md.Get("authorization") {
		if value == "Bearer "+s.srv.authToken {
			return true
		}
	}
	return false
}
