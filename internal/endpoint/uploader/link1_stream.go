package uploader

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
)

type StreamUploader struct {
	manager string
	timeout time.Duration
	token   string
}

type streamUploadResult struct {
	OK              bool   `json:"ok"`
	Message         string `json:"message,omitempty"`
	BatchID         string `json:"batch_id,omitempty"`
	AcceptedEvents  uint64 `json:"accepted_events,omitempty"`
	AcceptedSignals uint64 `json:"accepted_signals,omitempty"`
}

func NewStreamUploader(manager string) *StreamUploader {
	return NewStreamUploaderWithTimeout(manager, 10*time.Second)
}

func NewStreamUploaderWithTimeout(manager string, timeout time.Duration) *StreamUploader {
	return NewStreamUploaderWithOptions(manager, timeout, "")
}

func NewStreamUploaderWithOptions(manager string, timeout time.Duration, token string) *StreamUploader {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &StreamUploader{manager: normalizeGRPCAddress(manager), timeout: timeout, token: token}
}

func (u *StreamUploader) Upload(batch *analyticsv1.UploadBatch) (*analyticsv1.UploadAck, error) {
	data, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(batch)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), u.timeout)
	defer cancel()
	if u.token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", u.token)
	}
	conn, err := grpc.DialContext(ctx, u.manager, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stream, err := analyticsv1.NewLink1Client(conn).Stream(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&analyticsv1.StreamFrame{Type: "upload", PayloadJson: data}); err != nil {
		return nil, err
	}
	frame, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	if frame.GetType() != "upload" {
		return nil, fmt.Errorf("unexpected stream response type %q", frame.GetType())
	}
	var result streamUploadResult
	if err := json.Unmarshal(frame.GetPayloadJson(), &result); err != nil {
		return nil, fmt.Errorf("decode stream upload result: %w", err)
	}
	ack := &analyticsv1.UploadAck{
		Ok:              result.OK,
		Message:         result.Message,
		AcceptedEvents:  result.AcceptedEvents,
		AcceptedSignals: result.AcceptedSignals,
		BatchId:         result.BatchID,
	}
	if !ack.GetOk() {
		return ack, fmt.Errorf("upload rejected: %s", ack.GetMessage())
	}
	return ack, nil
}
