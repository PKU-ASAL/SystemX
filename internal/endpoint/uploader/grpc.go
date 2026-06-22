package uploader

import (
	"context"
	"fmt"
	"strings"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type GRPCUploader struct {
	manager string
	timeout time.Duration
	token   string
	tls     tlsconfig.ClientConfig
}

func NewGRPCUploader(manager string) *GRPCUploader {
	return NewGRPCUploaderWithTimeout(manager, 10*time.Second)
}

func NewGRPCUploaderWithTimeout(manager string, timeout time.Duration) *GRPCUploader {
	return NewGRPCUploaderWithOptions(manager, timeout, "")
}

func NewGRPCUploaderWithOptions(manager string, timeout time.Duration, token string) *GRPCUploader {
	return NewGRPCUploaderWithTLS(manager, timeout, token, tlsconfig.ClientConfig{})
}

func NewGRPCUploaderWithTLS(manager string, timeout time.Duration, token string, tlsCfg tlsconfig.ClientConfig) *GRPCUploader {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &GRPCUploader{manager: normalizeGRPCAddress(manager), timeout: timeout, token: token, tls: tlsCfg}
}

func (u *GRPCUploader) AppendBatch(batch *dataplanev1.DataBatch) (*dataplanev1.DataAck, error) {
	ctx, cancel := context.WithTimeout(context.Background(), u.timeout)
	defer cancel()
	if u.token != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-sysarmor-agent-token", u.token)
	}
	creds, err := tlsconfig.ClientCredentials(u.tls)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.DialContext(ctx, u.manager, grpc.WithTransportCredentials(creds), grpc.WithBlock())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ack, err := dataplanev1.NewAgentDataPlaneServiceClient(conn).AppendBatch(ctx, batch)
	if err != nil {
		return nil, err
	}
	if !AckCommitted(ack) {
		return ack, fmt.Errorf("append batch rejected: %s", ack.GetMessage())
	}
	return ack, nil
}

func normalizeGRPCAddress(manager string) string {
	manager = strings.TrimPrefix(manager, "http://")
	manager = strings.TrimPrefix(manager, "https://")
	return strings.TrimRight(manager, "/")
}
