package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/sysarmor/sysarmor-next-project/internal/managerapi"
	"github.com/sysarmor/sysarmor-next-project/internal/store/backend"
)

var version = "dev"

func main() {
	listen := flag.String("listen", ":9443", "manager HTTP listen address")
	storeBackend := flag.String("store-backend", backend.KindPostgres, "store backend: postgres")
	storePath := flag.String("store", "", "deprecated: file store path is not used by product backends")
	postgresDriver := flag.String("postgres-driver", envDefault("SYSARMOR_POSTGRES_DRIVER", "postgres"), "database/sql driver name for postgres backend")
	postgresDSN := flag.String("postgres-dsn", envDefault("SYSARMOR_POSTGRES_DSN", ""), "Postgres DSN for postgres backend")
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
	managerSrv := managerapi.NewServerWithOperatorToken(st, *operatorToken)

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
