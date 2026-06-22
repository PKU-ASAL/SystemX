package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/endpoint/dataappend"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	manager := flag.String("manager", "127.0.0.1:9444", "manager gRPC address")
	token := flag.String("token", "", "agent token")
	input := flag.String("input", "", "DataBatch protojson file")
	tlsCA := flag.String("tls-ca", "", "CA bundle used to verify manager gRPC")
	tlsCert := flag.String("tls-cert", "", "agent client certificate for mTLS")
	tlsKey := flag.String("tls-key", "", "agent client private key for mTLS")
	tlsServerName := flag.String("tls-server-name", "", "optional manager certificate SAN override")
	tlsInsecure := flag.Bool("tls-insecure", false, "use insecure gRPC transport")
	timeout := flag.Duration("timeout", 10*time.Second, "data append request timeout")
	flag.Parse()
	if *input == "" {
		fmt.Fprintln(os.Stderr, "--input is required")
		os.Exit(2)
	}
	batch, err := readDataBatch(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read batch: %v\n", err)
		os.Exit(1)
	}
	up := dataappend.NewGRPCAppenderWithTLS(*manager, *timeout, *token, tlsconfig.ClientConfig{
		CAFile:     *tlsCA,
		CertFile:   *tlsCert,
		KeyFile:    *tlsKey,
		ServerName: *tlsServerName,
		Insecure:   *tlsInsecure,
	})
	ack, err := up.AppendBatch(batch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "data_plane: %v\n", err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"batch_id":         ack.GetBatchId(),
		"accepted":         ack.GetAccepted(),
		"status":           ack.GetStatus().String(),
		"message":          ack.GetMessage(),
		"reason_code":      ack.GetReasonCode(),
		"committed_cursor": ack.GetCommittedCursor(),
		"accepted_events":  ack.GetAcceptedEvents(),
		"accepted_signals": ack.GetAcceptedSignals(),
		"retryable":        ack.GetRetryable(),
		"retry_after_ms":   ack.GetRetryAfterMs(),
		"contract_version": ack.GetContractVersion(),
	})
}

func readDataBatch(path string) (*dataplanev1.DataBatch, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var protoBatch dataplanev1.DataBatch
	if err := protojson.Unmarshal(data, &protoBatch); err != nil {
		return nil, err
	}
	if protoBatch.GetHeader() == nil {
		return nil, fmt.Errorf("DataBatch.header is required")
	}
	if protoBatch.Header.CreatedAtUnixNano == 0 {
		protoBatch.Header.CreatedAtUnixNano = time.Now().UnixNano()
	}
	return &protoBatch, nil
}
