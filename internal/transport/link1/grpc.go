package link1

import (
	"context"
	"encoding/json"
	"errors"
	"io"

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
	result, err := s.srv.AcceptUploadWithTransport(batch, "grpc")
	if err != nil {
		if errors.Is(err, ErrInvalidUpload) {
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		}
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

func (s *grpcServer) Stream(stream analyticsv1.Link1_StreamServer) error {
	if !s.authorized(stream.Context()) {
		return status.Error(codes.Unauthenticated, "unauthorized")
	}
	for {
		frame, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "recv stream frame: %v", err)
		}
		out, err := s.acceptStreamFrame(frame)
		if err != nil {
			out = &analyticsv1.StreamFrame{
				Type:        frame.GetType(),
				PayloadJson: mustJSON(UplinkFrameResult{Type: frame.GetType(), OK: false, Message: err.Error()}),
			}
		}
		if err := stream.Send(out); err != nil {
			return status.Errorf(codes.Internal, "send stream frame: %v", err)
		}
	}
}

func (s *grpcServer) acceptStreamFrame(frame *analyticsv1.StreamFrame) (*analyticsv1.StreamFrame, error) {
	if frame == nil {
		return nil, status.Error(codes.InvalidArgument, "stream frame is nil")
	}
	switch frame.GetType() {
	case "hello":
		var hello struct {
			TenantID      string `json:"tenant_id"`
			AgentID       string `json:"agent_id"`
			ScopeType     string `json:"scope_type,omitempty"`
			ScopeSelector string `json:"scope_selector,omitempty"`
		}
		if err := json.Unmarshal(frame.GetPayloadJson(), &hello); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "decode hello frame: %v", err)
		}
		if hello.AgentID == "" {
			return nil, status.Error(codes.InvalidArgument, "agent_id is required")
		}
		if hello.TenantID == "" {
			hello.TenantID = "default"
		}
		policy, _ := s.srv.store.EffectivePolicy(hello.TenantID, hello.AgentID, hello.ScopeType, hello.ScopeSelector)
		frames := []DownlinkFrame{resumeFrame(s.srv.resumeCursor(hello.TenantID, hello.AgentID)), policyUpdateFrame(policy)}
		for _, cmd := range s.srv.store.PendingResponses(hello.TenantID, hello.AgentID) {
			frames = append(frames, responseCommandFrame(cmd))
		}
		for _, req := range s.srv.store.PendingEvidencePullbacks(hello.TenantID, hello.AgentID) {
			frames = append(frames, evidencePullbackFrame(req))
		}
		return &analyticsv1.StreamFrame{Type: "downlink", PayloadJson: mustJSON(map[string]any{"frames": frames})}, nil
	default:
		result, err := s.srv.acceptUplinkFrameWithTransport(UplinkFrame{Type: frame.GetType(), Payload: json.RawMessage(frame.GetPayloadJson())}, "stream")
		if err != nil {
			return nil, err
		}
		return &analyticsv1.StreamFrame{Type: frame.GetType(), PayloadJson: mustJSON(result)}, nil
	}
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"ok":false,"message":"encode stream frame failed"}`)
	}
	return data
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
