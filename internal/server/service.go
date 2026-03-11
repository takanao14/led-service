package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	imagev1 "github.com/takanao14/led-image-api/gen/go/image/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	failedFileSuffix  = "-failed"
	timeoutFileSuffix = "-timeout"
)

type displayRequest struct {
	tempFilePath string
	duration     int32
}

type imageServiceServer struct {
	imagev1.UnimplementedImageServiceServer
	displayScript       string
	tempDir             string
	workerScriptTimeout time.Duration
	runDisplayScriptFn  func(context.Context, string, int32) error
	queue               chan displayRequest
}

func (s *imageServiceServer) executeDisplayScript(ctx context.Context, tempFilePath string, duration int32) error {
	if s.runDisplayScriptFn != nil {
		return s.runDisplayScriptFn(ctx, tempFilePath, duration)
	}
	return s.runDisplayScript(ctx, tempFilePath, duration)
}

func (s *imageServiceServer) startWorker(ctx context.Context) {
	slog.Info("starting display worker")
	for {
		select {
		case req, ok := <-s.queue:
			if !ok {
				slog.Info("display worker queue closed, shutting down")
				return
			}
			slog.Info("processing display request from queue", "path", req.tempFilePath, "duration", req.duration, "timeout", s.workerScriptTimeout)

			runCtx, cancel := context.WithTimeout(ctx, s.workerScriptTimeout)
			err := s.executeDisplayScript(runCtx, req.tempFilePath, req.duration)
			cancel()

			if err != nil {
				reason := "failed"
				if errors.Is(err, context.DeadlineExceeded) {
					reason = "timeout"
					slog.Error("display script timed out in worker", "path", req.tempFilePath, "timeout", s.workerScriptTimeout, "error", err)
				} else {
					slog.Error("display script failed in worker", "path", req.tempFilePath, "error", err)
				}
				markFailedTempFile(req.tempFilePath, reason)
				continue
			}

			slog.Info("display completed, removing temp file", "path", req.tempFilePath)
			removeTempFile(req.tempFilePath)
		case <-ctx.Done():
			slog.Info("display worker context canceled, shutting down")
			return
		}
	}
}

func markFailedTempFile(path string, reason string) {
	suffix := failedFileSuffix
	if reason == "timeout" {
		suffix = timeoutFileSuffix
	}

	ext := filepath.Ext(path)
	failedPath := strings.TrimSuffix(path, ext) + suffix + ext
	if failedPath == path {
		return
	}
	if err := os.Rename(path, failedPath); err != nil {
		slog.Warn("failed to preserve temp file", "path", path, "reason", reason, "error", err)
		return
	}
	slog.Warn("preserved temp file for investigation", "path", failedPath, "reason", reason)
}

func (s *imageServiceServer) SendImage(_ context.Context, req *imagev1.SendImageRequest) (*imagev1.SendImageResponse, error) {
	img := req.GetImage()
	if img == nil {
		slog.Error("received nil image")
		return nil, status.Error(codes.InvalidArgument, "image is required")
	}

	imageData := img.GetImageData()
	mimeType := img.GetMimeType()
	duration, err := validateDuration(req.GetDurationSeconds())
	if err != nil {
		slog.Warn("invalid request duration", "duration", req.GetDurationSeconds(), "error", err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if len(imageData) == 0 {
		slog.Error("received empty image data")
		return nil, status.Error(codes.InvalidArgument, "image data is empty")
	}

	tempFilePath, err := s.saveTempImage(imageData, mimeType)
	if err != nil {
		slog.Error("failed to save temp image", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to save temp image: %v", err)
	}

	slog.Info("saved image to temp file, queuing request", "path", tempFilePath, "duration", duration)

	select {
	case s.queue <- displayRequest{tempFilePath: tempFilePath, duration: duration}:
		return &imagev1.SendImageResponse{
			Success: true,
			Message: fmt.Sprintf("request accepted and queued (duration: %d seconds)", duration),
		}, nil
	default:
		// Queue is full (if buffered) or no worker is listening.
		// For now, let's assume it's full and return an error to avoid leaking temp files.
		removeTempFile(tempFilePath)
		slog.Error("display queue is full, rejecting request")
		return nil, status.Error(codes.ResourceExhausted, "display queue is full, please try again later")
	}
}

func validateDuration(duration int32) (int32, error) {
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be greater than 0")
	}
	return duration, nil
}
