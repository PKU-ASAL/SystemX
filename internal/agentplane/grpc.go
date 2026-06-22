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
			return dataAck(batch, dataplanev1.DataAck_STATUS_REJECTED, DataAckReasonInvalidUpload, err.Error(), false, 0, UploadResult{}), nil
		}
		statusCode := status.Code(err)
		retryable := dataAckRetryable(statusCode)
		if statusCode == codes.OK {
			statusCode = codes.Internal
			retryable = true
		}
		ackStatus := dataplanev1.DataAck_STATUS_REJECTED
		reason := DataAckReasonServerError
		retryAfter := uint64(0)
		if retryable {
			ackStatus = dataplanev1.DataAck_STATUS_RETRYABLE
			reason = DataAckReasonRetryableServerError
			retryAfter = 1000
		}
		return dataAck(batch, ackStatus, reason, err.Error(), retryable, retryAfter, UploadResult{}), nil
	}
	ackStatus := dataplanev1.DataAck_STATUS_ACCEPTED
	message := "accepted"
	if result.Duplicate {
		ackStatus = dataplanev1.DataAck_STATUS_DUPLICATE
		message = "duplicate"
	}
	return dataAck(batch, ackStatus, stringReasonCode(ackStatus), message, false, 0, result), nil
}

func dataAck(batch *dataplanev1.DataBatch, ackStatus dataplanev1.DataAck_Status, reason, message string, retryable bool, retryAfterMs uint64, result UploadResult) *dataplanev1.DataAck {
	batchID := ""
	if batch != nil && batch.GetHeader() != nil {
		batchID = batch.GetHeader().GetBatchId()
	}
	accepted := ackStatus == dataplanev1.DataAck_STATUS_ACCEPTED || ackStatus == dataplanev1.DataAck_STATUS_DUPLICATE
	return &dataplanev1.DataAck{
		BatchId:         batchID,
		Accepted:        accepted,
		Status:          ackStatus,
		Message:         message,
		ReasonCode:      reason,
		CommittedCursor: batchID,
		ServerTime:      time.Now().UTC().Format(time.RFC3339Nano),
		AcceptedEvents:  uint64(result.AcceptedEvents),
		AcceptedSignals: uint64(result.AcceptedSignals),
		Retryable:       retryable,
		RetryAfterMs:    retryAfterMs,
		Partial:         result.AcceptedEvents > 0 || result.AcceptedSignals > 0,
		ContractVersion: "dataplane.v1",
	}
}

func dataAckRetryable(code codes.Code) bool {
	switch code {
	case codes.Unavailable, codes.ResourceExhausted, codes.DeadlineExceeded, codes.Aborted, codes.Internal, codes.Unknown:
		return true
	default:
		return false
	}
}

func stringReasonCode(status dataplanev1.DataAck_Status) string {
	switch status {
	case dataplanev1.DataAck_STATUS_ACCEPTED:
		return DataAckReasonAccepted
	case dataplanev1.DataAck_STATUS_DUPLICATE:
		return DataAckReasonDuplicate
	case dataplanev1.DataAck_STATUS_RETRYABLE:
		return DataAckReasonRetryable
	case dataplanev1.DataAck_STATUS_REJECTED:
		return DataAckReasonRejected
	default:
		return DataAckReasonUnspecified
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
