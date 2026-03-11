package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestStartWorker_RemovesTempFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.png")
	writeTempTestFile(t, path)

	srv := &imageServiceServer{
		workerScriptTimeout: 200 * time.Millisecond,
		runDisplayScriptFn: func(context.Context, string, int32) error {
			return nil
		},
		queue: make(chan displayRequest, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		srv.startWorker(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	srv.queue <- displayRequest{tempFilePath: path, duration: 1}
	waitForCondition(t, time.Second, func() bool {
		_, err := os.Stat(path)
		return os.IsNotExist(err)
	})
}

func TestStartWorker_PreservesTempFileWithFailedSuffixOnScriptError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "failed.png")
	writeTempTestFile(t, path)

	srv := &imageServiceServer{
		workerScriptTimeout: 200 * time.Millisecond,
		runDisplayScriptFn: func(context.Context, string, int32) error {
			return errors.New("boom")
		},
		queue: make(chan displayRequest, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		srv.startWorker(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	srv.queue <- displayRequest{tempFilePath: path, duration: 1}
	renamed := filepath.Join(dir, "failed-failed.png")
	waitForCondition(t, time.Second, func() bool {
		_, err := os.Stat(renamed)
		return err == nil
	})
}

func TestStartWorker_PreservesTempFileWithTimeoutSuffixOnDeadlineExceeded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "timeout.png")
	writeTempTestFile(t, path)

	srv := &imageServiceServer{
		workerScriptTimeout: 20 * time.Millisecond,
		runDisplayScriptFn: func(ctx context.Context, _ string, _ int32) error {
			<-ctx.Done()
			return ctx.Err()
		},
		queue: make(chan displayRequest, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		srv.startWorker(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	srv.queue <- displayRequest{tempFilePath: path, duration: 1}
	renamed := filepath.Join(dir, "timeout-timeout.png")
	waitForCondition(t, time.Second, func() bool {
		_, err := os.Stat(renamed)
		return err == nil
	})
}

func TestStartWorker_UsesPerRequestTimeoutContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deadline.png")
	writeTempTestFile(t, path)

	deadlineObserved := make(chan time.Duration, 1)
	srv := &imageServiceServer{
		workerScriptTimeout: 100 * time.Millisecond,
		runDisplayScriptFn: func(ctx context.Context, _ string, _ int32) error {
			deadline, ok := ctx.Deadline()
			if !ok {
				return errors.New("deadline not set")
			}
			deadlineObserved <- time.Until(deadline)
			return nil
		},
		queue: make(chan displayRequest, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		srv.startWorker(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()

	srv.queue <- displayRequest{tempFilePath: path, duration: 1}

	select {
	case remaining := <-deadlineObserved:
		if remaining <= 0 || remaining > 150*time.Millisecond {
			t.Fatalf("unexpected deadline remaining: %v", remaining)
		}
	case <-time.After(time.Second):
		t.Fatal("did not observe worker deadline")
	}
}

func TestStartWorker_StopsOnParentContextCancel(t *testing.T) {
	srv := &imageServiceServer{
		workerScriptTimeout: 100 * time.Millisecond,
		runDisplayScriptFn: func(context.Context, string, int32) error {
			return nil
		},
		queue: make(chan displayRequest, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		srv.startWorker(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after parent context cancellation")
	}
}

func TestStartWorker_DoesNotProcessAfterCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "after-cancel.png")
	writeTempTestFile(t, path)

	var called atomic.Int32
	srv := &imageServiceServer{
		workerScriptTimeout: 100 * time.Millisecond,
		runDisplayScriptFn: func(context.Context, string, int32) error {
			called.Add(1)
			return nil
		},
		queue: make(chan displayRequest, 1),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		srv.startWorker(ctx)
		close(done)
	}()

	cancel()
	<-done

	srv.queue <- displayRequest{tempFilePath: path, duration: 1}
	time.Sleep(50 * time.Millisecond)

	if called.Load() != 0 {
		t.Fatalf("runDisplayScript called %d times after cancel, want 0", called.Load())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected queued file to remain untouched, stat err = %v", err)
	}
}

func writeTempTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", path, err)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
