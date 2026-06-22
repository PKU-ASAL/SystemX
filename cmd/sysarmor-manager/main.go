package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	controlplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/controlplane/v1"
	dataplanev1 "github.com/sysarmor/sysarmor-next-project/api/proto/dataplane/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/agentplane"
	"github.com/sysarmor/sysarmor-next-project/internal/managerapi"
	platformkafka "github.com/sysarmor/sysarmor-next-project/internal/platform/kafka"
	platformredis "github.com/sysarmor/sysarmor-next-project/internal/platform/redis"
	"github.com/sysarmor/sysarmor-next-project/internal/store/backend"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	ingestworker "github.com/sysarmor/sysarmor-next-project/internal/workers/ingest"
	"google.golang.org/grpc"
)

var version = "dev"

func main() {
	listen := flag.String("listen", ":9443", "manager HTTP listen address")
	grpcListen := flag.String("grpc-listen", ":9444", "manager agent data/control gRPC listen address")
	grpcTLSCert := flag.String("grpc-tls-cert", envDefault("SYSARMOR_GRPC_TLS_CERT", ""), "manager gRPC server TLS certificate")
	grpcTLSKey := flag.String("grpc-tls-key", envDefault("SYSARMOR_GRPC_TLS_KEY", ""), "manager gRPC server TLS private key")
	grpcClientCA := flag.String("grpc-client-ca", envDefault("SYSARMOR_GRPC_CLIENT_CA", ""), "CA bundle used to verify agent client certificates")
	grpcRequireClientCert := flag.Bool("grpc-require-client-cert", envDefault("SYSARMOR_GRPC_REQUIRE_CLIENT_CERT", "") == "true", "require and verify agent client certificates for gRPC")
	storeBackend := flag.String("store-backend", backend.KindPostgres, "store backend: postgres")
	storePath := flag.String("store", "", "deprecated: file store path is not used by product backends")
	postgresDriver := flag.String("postgres-driver", envDefault("SYSARMOR_POSTGRES_DRIVER", "postgres"), "database/sql driver name for postgres backend")
	postgresDSN := flag.String("postgres-dsn", envDefault("SYSARMOR_POSTGRES_DSN", ""), "Postgres DSN for postgres backend")
	kafkaBrokers := flag.String("kafka-brokers", envDefault("SYSARMOR_KAFKA_BROKERS", ""), "comma-separated Kafka brokers for raw telemetry ingest")
	redisAddr := flag.String("redis-addr", envDefault("SYSARMOR_REDIS_ADDR", ""), "Redis address for agent data hot state")
	localIngest := flag.Bool("local-ingest", false, "development/test mode: process accepted DataBatch payloads in-process")
	devToken := flag.String("dev-token", "", "static development agent token; empty disables token checks")
	operatorToken := flag.String("operator-token", "", "static development operator token for control-plane writes; empty disables operator checks")
	flag.Parse()

	if flag.NArg() > 0 && flag.Arg(0) == "version" {
		fmt.Println(version)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if *storeBackend == backend.KindFile {
		fmt.Fprintln(os.Stderr, "open store: file backend has been removed from the sysarmor-manager product path; use postgres")
		os.Exit(1)
	}
	storeResult, err := backend.Open(ctx, backend.Options{
		Kind:           *storeBackend,
		Path:           *storePath,
		PostgresDriver: *postgresDriver,
		PostgresDSN:    *postgresDSN,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := storeResult.Close(); err != nil {
			log.Printf("close store backend: %v", err)
		}
	}()
	st := storeResult.Store
	managerSrv := managerapi.NewServerWithTokens(st, *devToken, *operatorToken)
	if *localIngest {
		managerSrv.WithLocalProcessor(ingestworker.NewProcessor(st, nil))
	}
	if *kafkaBrokers != "" {
		producer, err := platformkafka.NewWriterProducer(splitCSV(*kafkaBrokers))
		if err != nil {
			fmt.Fprintf(os.Stderr, "open kafka producer: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if err := producer.Close(); err != nil {
				log.Printf("close kafka producer: %v", err)
			}
		}()
		managerSrv.WithProducer(producer)
	}
	if *redisAddr != "" {
		hotState, err := platformredis.NewClientHotState(*redisAddr, 2*time.Minute)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open redis hot state: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if err := hotState.Close(); err != nil {
				log.Printf("close redis hot state: %v", err)
			}
		}()
		managerSrv.WithHotState(hotState)
	}
	var grpcOptions []grpc.ServerOption
	grpcTLSOption, err := tlsconfig.ServerOption(*grpcTLSCert, *grpcTLSKey, *grpcClientCA, *grpcRequireClientCert)
	if err != nil {
		fmt.Fprintf(os.Stderr, "manager grpc tls: %v\n", err)
		os.Exit(1)
	}
	if grpcTLSOption != nil {
		grpcOptions = append(grpcOptions, grpcTLSOption)
	}
	grpcServer := grpc.NewServer(grpcOptions...)
	dataplanev1.RegisterAgentDataPlaneServiceServer(grpcServer, agentplane.NewDataServer(managerSrv))
	controlplanev1.RegisterAgentControlPlaneServiceServer(grpcServer, agentplane.NewControlServer(managerSrv))
	lis, err := net.Listen("tcp", *grpcListen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "manager grpc listen: %v\n", err)
		os.Exit(1)
	}
	go func() {
		log.Printf("sysarmor-manager agent data/control grpc listening on %s mtls=%t", *grpcListen, *grpcClientCA != "" || *grpcRequireClientCert)
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("manager grpc serve: %v", err)
		}
	}()

	srv := &http.Server{
		Addr:    *listen,
		Handler: managerSrv.Handler(),
	}
	log.Printf("sysarmor-manager listening on %s store_backend=%s store=%s", *listen, *storeBackend, *storePath)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "manager serve: %v\n", err)
		os.Exit(1)
	}
}

func envDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
