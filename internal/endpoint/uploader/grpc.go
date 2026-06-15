package uploader

import (
	"context"
	"fmt"
	"strings"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type GRPCUploader struct {
	manager string
	timeout time.Duration
	token   string
}

func NewGRPCUploader(manager string) *GRPCUploader {
	return NewGRPCUploaderWithTimeout(manager, 10*time.Second)
}

func NewGRPCUploaderWithTimeout(manager string, timeout time.Duration) *GRPCUploader {
	return NewGRPCUploaderWithOptions(manager, timeout, "")
}

func NewGRPCUploaderWithOptions(manager string, timeout time.Duration, token string) *GRPCUploader {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &GRPCUploader{manager: normalizeGRPCAddress(manager), timeout: timeout, token: token}
}

func (u *GRPCUploader) Upload(batch *analyticsv1.UploadBatch) (*analyticsv1.UploadAck, error) {
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
	ack, err := analyticsv1.NewLink1Client(conn).Upload(ctx, batch)
	if err != nil {
		return nil, err
	}
	if !ack.GetOk() {
		return ack, fmt.Errorf("upload rejected: %s", ack.GetMessage())
	}
	return ack, nil
}

func normalizeGRPCAddress(manager string) string {
	manager = strings.TrimPrefix(manager, "http://")
	manager = strings.TrimPrefix(manager, "https://")
	return strings.TrimRight(manager, "/")
}
