package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	imagev1 "github.com/takanao14/led-image-api/gen/go/image/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultAddr       = "127.0.0.1:50051"
	defaultTimeout    = 5 * time.Second
	jingleDuration    = 5 * time.Second
	timeoutSlack      = 10 * time.Second
	minimumRPCTimeout = 20 * time.Second
	envDebug          = "DEBUG"
)

func main() {
	configureLogger()

	if err := run(); err != nil {
		slog.Error("led-client failed", "error", err)
		os.Exit(1)
	}
}

func run() (err error) {
	addr := flag.String("addr", defaultAddr, "gRPC server address")
	imagePath := flag.String("file", "", "Path to image file")
	mimeType := flag.String("mime", "", "MIME type (optional, auto-detected when omitted)")
	duration := flag.Int("duration", 10, "Display duration in seconds")
	timeout := flag.Duration("timeout", defaultTimeout, "RPC timeout (0s = default 5s)")
	flag.Parse()
	slog.Debug("parsed flags", "addr", *addr, "file", *imagePath, "duration", *duration, "timeout", timeout.String())

	if strings.TrimSpace(*imagePath) == "" {
		return fmt.Errorf("missing required argument: -file")
	}
	if *duration <= 0 {
		return fmt.Errorf("invalid duration: %d", *duration)
	}

	effectiveTimeout := *timeout
	if effectiveTimeout <= 0 {
		effectiveTimeout = calculateRPCTimeout(*duration)
		slog.Info("using auto-calculated timeout", "timeout", effectiveTimeout, "duration_seconds", *duration)
	}
	slog.Debug("effective timeout resolved", "timeout", effectiveTimeout)

	data, readErr := os.ReadFile(*imagePath)
	if readErr != nil {
		return fmt.Errorf("failed to read image file %q: %w", *imagePath, readErr)
	}

	resolvedMime := strings.TrimSpace(*mimeType)
	if resolvedMime == "" {
		resolvedMime = detectMimeType(data, *imagePath)
	}
	slog.Debug("mime resolved", "mime", resolvedMime, "explicit_mime", strings.TrimSpace(*mimeType) != "")

	slog.Info("prepared image payload", "file", *imagePath, "bytes", len(data), "mime", resolvedMime)

	conn, dialErr := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if dialErr != nil {
		return fmt.Errorf("failed to create grpc client for %s: %w", *addr, dialErr)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("failed to close grpc connection: %w", closeErr)
				return
			}
			slog.Warn("failed to close grpc connection", "error", closeErr)
		}
	}()

	client := imagev1.NewImageServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), effectiveTimeout)
	defer cancel()
	slog.Debug("sending image request", "addr", *addr)

	resp, sendErr := client.SendImage(ctx, &imagev1.SendImageRequest{
		Image: &imagev1.ImageData{
			ImageData: data,
			MimeType:  resolvedMime,
		},
		DurationSeconds: int32(*duration),
	})
	if sendErr != nil {
		return fmt.Errorf("image request failed for %s: %w", *addr, sendErr)
	}

	slog.Info("image request completed", "success", resp.GetSuccess(), "message", resp.GetMessage())
	return nil
}

func configureLogger() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	if os.Getenv(envDebug) != "" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: true})
	}
	logger := slog.New(handler).With("service", "led-client")
	slog.SetDefault(logger)
	slog.Debug("debug logging enabled", "env", envDebug)
}

func detectMimeType(data []byte, path string) string {
	if len(data) > 0 {
		sniffLen := min(len(data), 512)
		sniffed := http.DetectContentType(data[:sniffLen])
		if sniffed != "application/octet-stream" {
			return sniffed
		}
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".ppm", ".pnm":
		return "image/x-portable-pixmap"
	default:
		return "application/octet-stream"
	}
}

func calculateRPCTimeout(displayDurationSeconds int) time.Duration {
	calculated := time.Duration(displayDurationSeconds)*time.Second + jingleDuration + timeoutSlack
	if calculated < minimumRPCTimeout {
		return minimumRPCTimeout
	}
	return calculated
}
