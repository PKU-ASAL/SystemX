package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/sysarmor/sysarmor-next-project/internal/gateway"
	platformkafka "github.com/sysarmor/sysarmor-next-project/internal/platform/kafka"
	platformredis "github.com/sysarmor/sysarmor-next-project/internal/platform/redis"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/store/backend"
	"github.com/sysarmor/sysarmor-next-project/internal/tlsconfig"
	ingestworker "github.com/sysarmor/sysarmor-next-project/internal/workers/ingest"
	"google.golang.org/grpc"
)

var version = "dev"

func main() {
	listen := flag.String("listen", ":9444", "gateway agent data/control gRPC listen address")
	grpcTLSCert := flag.String("tls-cert", envDefault("SYSARMOR_GRPC_TLS_CERT", ""), "gateway gRPC server TLS certificate")
	grpcTLSKey := flag.String("tls-key", envDefault("SYSARMOR_GRPC_TLS_KEY", ""), "gateway gRPC server TLS private key")
	grpcClientCA := flag.String("client-ca", envDefault("SYSARMOR_GRPC_CLIENT_CA", ""), "CA bundle used to verify agent client certificates")
	grpcRequireClientCert := flag.Bool("require-client-cert", envDefault("SYSARMOR_GRPC_REQUIRE_CLIENT_CERT", "") == "true", "require and verify agent client certificates")
	storeBackend := flag.String("store-backend", backend.KindPostgres, "store backend: postgres")
	postgresDriver := flag.String("postgres-driver", envDefault("SYSARMOR_POSTGRES_DRIVER", "postgres"), "database/sql driver name for postgres backend")
	postgresDSN := flag.String("postgres-dsn", envDefault("SYSARMOR_POSTGRES_DSN", ""), "Postgres DSN for postgres backend")
	kafkaBrokers := flag.String("kafka-brokers", envDefault("SYSARMOR_KAFKA_BROKERS", ""), "comma-separated Kafka brokers for raw telemetry ingest")
	redisAddr := flag.String("redis-addr", envDefault("SYSARMOR_REDIS_ADDR", ""), "Redis address for agent session hot state")
	localIngest := flag.Bool("local-ingest", false, "development/test mode: process accepted DataBatch payloads in-process")
	devToken := flag.String("dev-token", "", "static development agent token; empty disables token checks")
	flag.Parse()

	if flag.NArg() > 0 && flag.Arg(0) == "version" {
		fmt.Println(version)
		return
	}
	if *storeBackend == backend.KindFile {
		fmt.Fprintln(os.Stderr, "open store: file backend has been removed from the sysarmor-gateway product path; use postgres")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	openCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	storeResult, err := backend.Open(openCtx, backend.Options{
		Kind:           *storeBackend,
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

	runtime, cleanup := openGatewayRuntime(ctx, gatewayRuntimeConfig{
		store:       storeResult.Store,
		kafka:       *kafkaBrokers,
		redis:       *redisAddr,
		localIngest: *localIngest,
		agentToken:  *devToken,
	})
	defer cleanup()

	var grpcOptions []grpc.ServerOption
	grpcTLSOption, err := tlsconfig.ServerOption(*grpcTLSCert, *grpcTLSKey, *grpcClientCA, *grpcRequireClientCert)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gateway grpc tls: %v\n", err)
		os.Exit(1)
	}
	if grpcTLSOption != nil {
		grpcOptions = append(grpcOptions, grpcTLSOption)
	}
	grpcServer := grpc.NewServer(grpcOptions...)
	gateway.RegisterAgentServices(grpcServer, runtime)

	lis, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gateway grpc listen: %v\n", err)
		os.Exit(1)
	}
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	log.Printf("sysarmor-gateway listening on %s mtls=%t local_ingest=%t", *listen, *grpcClientCA != "" || *grpcRequireClientCert, *localIngest)
	if err := grpcServer.Serve(lis); err != nil {
		fmt.Fprintf(os.Stderr, "gateway grpc serve: %v\n", err)
		os.Exit(1)
	}
}

type gatewayRuntimeConfig struct {
	store       *store.Store
	kafka       string
	redis       string
	localIngest bool
	agentToken  string
}

func openGatewayRuntime(ctx context.Context, cfg gatewayRuntimeConfig) (*gateway.Runtime, func()) {
	cleanup := func() {}
	var producer platformkafka.Producer = platformkafka.NoopProducer{}
	if cfg.kafka != "" {
		next, err := platformkafka.NewWriterProducer(splitCSV(cfg.kafka))
		if err != nil {
			fmt.Fprintf(os.Stderr, "open kafka producer: %v\n", err)
			os.Exit(1)
		}
		producer = next
		cleanup = appendCleanup(cleanup, func() {
			if err := next.Close(); err != nil {
				log.Printf("close kafka producer: %v", err)
			}
		})
	}

	var hotState platformredis.HotState = platformredis.NoopHotState{}
	if cfg.redis != "" {
		next, err := platformredis.NewClientHotState(cfg.redis, 2*time.Minute)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open redis hot state: %v\n", err)
			os.Exit(1)
		}
		hotState = next
		cleanup = appendCleanup(cleanup, func() {
			if err := next.Close(); err != nil {
				log.Printf("close redis hot state: %v", err)
			}
		})
	}

	var processor *ingestworker.Processor
	if cfg.localIngest {
		processor = ingestworker.NewProcessor(cfg.store, nil)
	}

	return gateway.NewRuntime(gateway.RuntimeOptions{
		Store:          cfg.store,
		Producer:       producer,
		HotState:       hotState,
		LocalProcessor: processor,
		AgentToken:     cfg.agentToken,
	}), cleanup
}

func appendCleanup(first, next func()) func() {
	return func() {
		next()
		first()
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
