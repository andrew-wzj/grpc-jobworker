package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"

	"github.com/andrew-wzj/grpc-jobworker/internal/grpcapi"
	"github.com/andrew-wzj/grpc-jobworker/internal/httpapi"
	"github.com/andrew-wzj/grpc-jobworker/internal/tlsconfig"
	"github.com/andrew-wzj/grpc-jobworker/internal/worker"
	"github.com/andrew-wzj/grpc-jobworker/web"
)

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	grpcAddr := fs.String("grpc-addr", "127.0.0.1:50051", "gRPC listen address")
	httpAddr := fs.String("http-addr", "127.0.0.1:8080", "HTTP (REST + web UI) listen address; empty disables HTTP")
	dataDir := fs.String("data-dir", "data", "directory for the job database and logs")
	maxConc := fs.Int("max-concurrent", 16, "maximum number of jobs running at once")
	token := fs.String("token", os.Getenv("JOBWORKER_TOKEN"), "API token required from clients (default $JOBWORKER_TOKEN)")
	tlsCert := fs.String("tls-cert", "", "gRPC server certificate (enables TLS)")
	tlsKey := fs.String("tls-key", "", "gRPC server private key")
	clientCA := fs.String("tls-client-ca", "", "CA that signs client certificates (enables mutual TLS)")
	fs.Parse(args)

	if *token == "" {
		for _, addr := range []string{*grpcAddr, *httpAddr} {
			if addr != "" && !isLoopback(addr) {
				return fmt.Errorf("refusing to listen on %s without -token: anyone who can reach it could run commands on this machine", addr)
			}
		}
	}
	if (*tlsCert == "") != (*tlsKey == "") {
		return errors.New("-tls-cert and -tls-key must be set together")
	}
	if *clientCA != "" && *tlsCert == "" {
		return errors.New("-tls-client-ca requires -tls-cert and -tls-key")
	}

	w, err := worker.New(worker.Config{DataDir: *dataDir, MaxConcurrent: *maxConc})
	if err != nil {
		return err
	}

	var opts []grpc.ServerOption
	if *tlsCert != "" {
		cfg, err := tlsconfig.Server(*tlsCert, *tlsKey, *clientCA)
		if err != nil {
			return err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(cfg)))
	}
	if *token != "" {
		unary, stream := grpcapi.TokenAuth(*token)
		opts = append(opts, grpc.UnaryInterceptor(unary), grpc.StreamInterceptor(stream))
	}
	grpcServer := grpc.NewServer(opts...)
	grpcapi.Register(grpcServer, w)
	reflection.Register(grpcServer) // lets tools like grpcurl discover the API

	grpcLis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		return err
	}
	errc := make(chan error, 2)
	go func() { errc <- grpcServer.Serve(grpcLis) }()
	mode := "plaintext"
	if *clientCA != "" {
		mode = "mutual TLS"
	} else if *tlsCert != "" {
		mode = "TLS"
	}
	log.Printf("gRPC listening on %s (%s)", grpcLis.Addr(), mode)

	var httpServer *http.Server
	if *httpAddr != "" {
		httpServer = &http.Server{
			Addr:              *httpAddr,
			Handler:           httpapi.New(w, *token, web.Files),
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() { errc <- httpServer.ListenAndServe() }()
		log.Printf("HTTP listening on http://%s", *httpAddr)
	}
	if *token == "" {
		log.Printf("no -token set: API is open to anyone on this machine")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server error: %v", err)
		}
	case <-ctx.Done():
		log.Printf("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if httpServer != nil {
		_ = httpServer.Shutdown(shutdownCtx)
	}
	// Stop jobs first so open StreamOutput calls finish, then stop gRPC.
	werr := w.Shutdown(shutdownCtx)
	stopped := make(chan struct{})
	go func() { grpcServer.GracefulStop(); close(stopped) }()
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}
	return werr
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
