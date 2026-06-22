package agentplane

import (
	"context"
	"errors"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type grpcServer struct {
	analyticsv1.UnimplementedAgentDataServiceServer
	backend Backend
}

func NewDataServer(backend Backend) analyticsv1.AgentDataServiceServer {
	return &grpcServer{backend: backend}
}

func (s *grpcServer) Upload(ctx context.Context, batch *dataplanev1.DataBatch) (*dataplanev1.DataAck, error) {
	if !s.authorized(ctx) {
		return nil, status.Error(codes.Unauthenticated, "unauthorized")
	}
	peerID, hasPeer, err := validatePeerDataIdentity(ctx, batch.GetHeader())
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, err.Error())
	}
	if hasPeer {
		header := batch.GetHeader()
		agent := agentIdentityFromPeer(peerID, header.GetHostId(), header.GetLabels()["agent_version"])
		if err := s.backend.BindAgentIdentity(agent); err != nil {
			return nil, status.Error(codes.PermissionDenied, err.Error())
		}
	}
	result, err := s.backend.AcceptUploadWithTransport(batch, "grpc")
	if err != nil {
		if errors.Is(err, ErrInvalidUpload) {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "accept upload: %v", err)
	}
	status := dataplanev1.DataAck_STATUS_ACCEPTED
	message := "accepted"
	if result.Duplicate {
		status = dataplanev1.DataAck_STATUS_DUPLICATE
		message = "duplicate"
	}
	return &dataplanev1.DataAck{
		BatchId:         batch.GetHeader().GetBatchId(),
		Accepted:        true,
		Status:          status,
		Message:         message,
		ReasonCode:      stringReasonCode(status),
		CommittedCursor: batch.GetHeader().GetBatchId(),
		ServerTime:      time.Now().UTC().Format(time.RFC3339Nano),
		AcceptedEvents:  uint64(result.AcceptedEvents),
		AcceptedSignals: uint64(result.AcceptedSignals),
		ContractVersion: "dataplane.v1",
	}, nil
}

func stringReasonCode(status dataplanev1.DataAck_Status) string {
	switch status {
	case dataplanev1.DataAck_STATUS_ACCEPTED:
		return "accepted"
	case dataplanev1.DataAck_STATUS_DUPLICATE:
		return "duplicate"
	case dataplanev1.DataAck_STATUS_RETRYABLE:
		return "retryable"
	case dataplanev1.DataAck_STATUS_REJECTED:
		return "rejected"
	default:
		return "unspecified"
	}
}

func (s *grpcServer) authorized(ctx context.Context) bool {
	token := s.backend.AgentToken()
	if token == "" {
		return true
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}
	for _, value := range md.Get("x-sysarmor-agent-token") {
		if value == token {
			return true
		}
	}
	for _, value := range md.Get("authorization") {
		if value == "Bearer "+token {
			return true
		}
	}
	return false
}
