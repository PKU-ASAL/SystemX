package uploader

import (
	"context"
	"fmt"
	"strings"
	"time"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GRPCUploader struct {
	manager string
	timeout time.Duration
}

func NewGRPCUploader(manager string) *GRPCUploader {
	return NewGRPCUploaderWithTimeout(manager, 10*time.Second)
}

func NewGRPCUploaderWithTimeout(manager string, timeout time.Duration) *GRPCUploader {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &GRPCUploader{manager: normalizeGRPCAddress(manager), timeout: timeout}
}

func (u *GRPCUploader) Upload(batch *analyticsv1.UploadBatch) error {
	ctx, cancel := context.WithTimeout(context.Background(), u.timeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, u.manager, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer conn.Close()
	ack, err := analyticsv1.NewLink1Client(conn).Upload(ctx, batch)
	if err != nil {
		return err
	}
	if !ack.GetOk() {
		return fmt.Errorf("upload rejected: %s", ack.GetMessage())
	}
	return nil
}

func normalizeGRPCAddress(manager string) string {
	manager = strings.TrimPrefix(manager, "http://")
	manager = strings.TrimPrefix(manager, "https://")
	return strings.TrimRight(manager, "/")
}
