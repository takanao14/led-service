package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	imagev1 "github.com/takanao14/led-image-api/gen/go/image/v1"
	"google.golang.org/grpc"
)

func Run() error {
	configureLogger()

	cfg, err := resolveConfig()
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := os.RemoveAll(cfg.tempDir); removeErr != nil {
			slog.Warn("failed to remove temp directory", "path", cfg.tempDir, "error", removeErr)
		}
	}()

	slog.Info("starting gRPC server", "addr", cfg.listenAddr, "display_script", cfg.displayScript, "temp_dir", cfg.tempDir, "worker_script_timeout", cfg.workerScriptTimeout)

	lis, err := net.Listen("tcp", cfg.listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.listenAddr, err)
	}

	grpcServer := grpc.NewServer()
	srv := &imageServiceServer{
		displayScript:       cfg.displayScript,
		tempDir:             cfg.tempDir,
		workerScriptTimeout: cfg.workerScriptTimeout,
		queue:               make(chan displayRequest, 10), // Buffer up to 10 requests
	}
	imagev1.RegisterImageServiceServer(grpcServer, srv)

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	go srv.startWorker(workerCtx)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	serveErrCh := make(chan error, 1)
	go func() {
		slog.Info("gRPC server listening", "addr", cfg.listenAddr)
		serveErrCh <- grpcServer.Serve(lis)
	}()

	select {
	case <-stop:
		slog.Info("shutting down gRPC server...")
		grpcServer.GracefulStop()
		if serveErr := <-serveErrCh; serveErr != nil {
			return fmt.Errorf("grpc server stopped with error: %w", serveErr)
		}
		return nil
	case serveErr := <-serveErrCh:
		if serveErr != nil {
			return fmt.Errorf("failed to serve grpc: %w", serveErr)
		}
		return nil
	}
}
