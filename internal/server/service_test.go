package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	imagev1 "github.com/takanao14/led-image-api/gen/go/image/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestValidateDuration(t *testing.T) {
	tests := []struct {
		name      string
		duration  int32
		wantError bool
	}{
		{name: "negative", duration: -1, wantError: true},
		{name: "zero", duration: 0, wantError: true},
		{name: "positive", duration: 1, wantError: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateDuration(tc.duration)
			if tc.wantError {
				if err == nil {
					t.Fatalf("validateDuration(%d) expected error", tc.duration)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateDuration(%d) error = %v", tc.duration, err)
			}
			if got != tc.duration {
				t.Fatalf("validateDuration(%d) = %d, want %d", tc.duration, got, tc.duration)
			}
		})
	}
}

func TestMarkFailedTempFile_AppendsSuffixBeforeExtension(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "sample.png")
	if err := os.WriteFile(original, []byte("x"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", original, err)
	}

	markFailedTempFile(original, "failed")
	failedPath := filepath.Join(dir, "sample-failed.png")

	if _, err := os.Stat(failedPath); err != nil {
		t.Fatalf("expected renamed file at %q: %v", failedPath, err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("expected original file removed, stat err = %v", err)
	}
}

func TestMarkFailedTempFile_AppendsTimeoutSuffixBeforeExtension(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "sample.png")
	if err := os.WriteFile(original, []byte("x"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", original, err)
	}

	markFailedTempFile(original, "timeout")
	timeoutPath := filepath.Join(dir, "sample-timeout.png")

	if _, err := os.Stat(timeoutPath); err != nil {
		t.Fatalf("expected renamed file at %q: %v", timeoutPath, err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("expected original file removed, stat err = %v", err)
	}
}

func TestMarkFailedTempFile_HandlesFileWithoutExtension(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "sample")
	if err := os.WriteFile(original, []byte("x"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", original, err)
	}

	markFailedTempFile(original, "failed")
	failedPath := filepath.Join(dir, "sample-failed")

	if _, err := os.Stat(failedPath); err != nil {
		t.Fatalf("expected renamed file at %q: %v", failedPath, err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("expected original file removed, stat err = %v", err)
	}
}

func TestSendImage_ReturnsInvalidArgumentWhenImageNil(t *testing.T) {
	srv := &imageServiceServer{queue: make(chan displayRequest, 1)}

	_, err := srv.SendImage(context.Background(), &imagev1.SendImageRequest{DurationSeconds: 1})
	if err == nil {
		t.Fatal("SendImage expected error when image is nil")
	}
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Fatalf("status code = %v, want %v", code, codes.InvalidArgument)
	}
}

func TestSendImage_ReturnsInvalidArgumentWhenImageDataEmpty(t *testing.T) {
	srv := &imageServiceServer{queue: make(chan displayRequest, 1)}

	_, err := srv.SendImage(context.Background(), &imagev1.SendImageRequest{
		Image:           &imagev1.ImageData{MimeType: "image/png"},
		DurationSeconds: 1,
	})
	if err == nil {
		t.Fatal("SendImage expected error when image data is empty")
	}
	if code := status.Code(err); code != codes.InvalidArgument {
		t.Fatalf("status code = %v, want %v", code, codes.InvalidArgument)
	}
}

func TestSendImage_ReturnsResourceExhaustedWhenQueueFull(t *testing.T) {
	tempDir := t.TempDir()
	queue := make(chan displayRequest, 1)
	queue <- displayRequest{tempFilePath: filepath.Join(tempDir, "busy.png"), duration: 1}
	srv := &imageServiceServer{tempDir: tempDir, queue: queue}

	_, err := srv.SendImage(context.Background(), &imagev1.SendImageRequest{
		Image: &imagev1.ImageData{
			ImageData: []byte("img"),
			MimeType:  "image/png",
		},
		DurationSeconds: 1,
	})
	if err == nil {
		t.Fatal("SendImage expected queue full error")
	}
	if code := status.Code(err); code != codes.ResourceExhausted {
		t.Fatalf("status code = %v, want %v", code, codes.ResourceExhausted)
	}

	entries, readErr := os.ReadDir(tempDir)
	if readErr != nil {
		t.Fatalf("os.ReadDir(%q): %v", tempDir, readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected temporary file cleanup on queue-full path, got %d files", len(entries))
	}
}
