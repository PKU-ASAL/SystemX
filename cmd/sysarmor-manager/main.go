package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	analyticsv1 "github.com/sysarmor/sysarmor-next-project/api/proto/analytics/v1"
	"github.com/sysarmor/sysarmor-next-project/internal/store"
	"github.com/sysarmor/sysarmor-next-project/internal/transport/link1"
	"google.golang.org/grpc"
)

var version = "dev"

func main() {
	listen := flag.String("listen", ":9443", "manager HTTP listen address")
	grpcListen := flag.String("grpc-listen", ":9444", "manager Link1 gRPC listen address")
	storePath := flag.String("store", "/tmp/sysarmor-manager.json", "store path")
	devToken := flag.String("dev-token", "", "static development agent token; empty disables token checks")
	flag.Parse()

	if flag.NArg() > 0 && flag.Arg(0) == "version" {
		fmt.Println(version)
		return
	}

	st, err := store.Open(*storePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		os.Exit(1)
	}
	linkSrv := link1.NewServerWithAuth(st, *devToken)
	grpcServer := grpc.NewServer()
	analyticsv1.RegisterLink1Server(grpcServer, link1.NewGRPCServer(linkSrv))
	lis, err := net.Listen("tcp", *grpcListen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "manager grpc listen: %v\n", err)
		os.Exit(1)
	}
	go func() {
		log.Printf("sysarmor-manager link1 grpc listening on %s", *grpcListen)
		if err := grpcServer.Serve(lis); err != nil {
			log.Printf("manager grpc serve: %v", err)
		}
	}()

	srv := &http.Server{
		Addr:    *listen,
		Handler: linkSrv.Handler(),
	}
	log.Printf("sysarmor-manager listening on %s store=%s", *listen, *storePath)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "manager serve: %v\n", err)
		os.Exit(1)
	}
}
