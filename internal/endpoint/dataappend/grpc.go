package dataappend

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

type GRPCAppender struct {
	manager string
	timeout time.Duration
	token   string
	tls     tlsconfig.ClientConfig
}

func NewGRPCAppender(manager string) *GRPCAppender {
	return NewGRPCAppenderWithTimeout(manager, 10*time.Second)
}

func NewGRPCAppenderWithTimeout(manager string, timeout time.Duration) *GRPCAppender {
	return NewGRPCAppenderWithOptions(manager, timeout, "")
}

func NewGRPCAppenderWithOptions(manager string, timeout time.Duration, token string) *GRPCAppender {
	return NewGRPCAppenderWithTLS(manager, timeout, token, tlsconfig.ClientConfig{})
}

func NewGRPCAppenderWithTLS(manager string, timeout time.Duration, token string, tlsCfg tlsconfig.ClientConfig) *GRPCAppender {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &GRPCAppender{manager: normalizeGRPCAddress(manager), timeout: timeout, token: token, tls: tlsCfg}
}

func (u *GRPCAppender) AppendBatch(batch *dataplanev1.DataBatch) (*dataplanev1.DataAck, error) {
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
