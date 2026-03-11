package server

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogScriptLine_MapsSeverityHints(t *testing.T) {
	tests := []struct {
		name        string
		stream      string
		line        string
		wantLevel   slog.Level
		wantMessage string
	}{
		{
			name:        "error hint on stdout",
			stream:      "stdout",
			line:        "[display-script][ERROR] boom",
			wantLevel:   slog.LevelError,
			wantMessage: "display script output",
		},
		{
			name:        "warn hint on stdout",
			stream:      "stdout",
			line:        "[display-script][WARN] careful",
			wantLevel:   slog.LevelWarn,
			wantMessage: "display script output",
		},
		{
			name:        "stderr defaults to warn",
			stream:      "stderr",
			line:        "plain stderr line",
			wantLevel:   slog.LevelWarn,
			wantMessage: "display script output",
		},
		{
			name:        "stdout defaults to info",
			stream:      "stdout",
			line:        "plain stdout line",
			wantLevel:   slog.LevelInfo,
			wantMessage: "display script output",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			captured := make([]capturedLog, 0, 1)
			var mu sync.Mutex

			originalEmitLog := emitLog
			emitLog = func(_ context.Context, level slog.Level, msg string, args ...any) {
				mu.Lock()
				defer mu.Unlock()
				captured = append(captured, capturedLog{level: level, msg: msg, args: args})
			}
			defer func() {
				emitLog = originalEmitLog
			}()

			logScriptLine(tc.stream, tc.line, "/tmp/image.png")

			mu.Lock()
			defer mu.Unlock()
			if len(captured) != 1 {
				t.Fatalf("captured logs = %d, want 1", len(captured))
			}
			if captured[0].level != tc.wantLevel {
				t.Fatalf("log level = %v, want %v", captured[0].level, tc.wantLevel)
			}
			if captured[0].msg != tc.wantMessage {
				t.Fatalf("log message = %q, want %q", captured[0].msg, tc.wantMessage)
			}
			if !containsArgPair(captured[0].args, "line", tc.line) {
				t.Fatalf("expected args to contain line=%q, got %#v", tc.line, captured[0].args)
			}
		})
	}
}

func TestRunDisplayScript_ReturnsNilOnSuccess(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "display.sh")
	imagePath := filepath.Join(dir, "image.png")

	writeExecutableScript(t, scriptPath, "#!/bin/sh\necho '[display-script][INFO] ok'\nexit 0\n")
	if err := os.WriteFile(imagePath, []byte("img"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", imagePath, err)
	}

	srv := &imageServiceServer{displayScript: scriptPath}
	if err := srv.runDisplayScript(context.Background(), imagePath, 1); err != nil {
		t.Fatalf("runDisplayScript() error = %v", err)
	}
}

func TestRunDisplayScript_ReturnsWrappedErrorOnStartFailure(t *testing.T) {
	srv := &imageServiceServer{displayScript: filepath.Join(t.TempDir(), "missing-script.sh")}

	err := srv.runDisplayScript(context.Background(), "/tmp/image.png", 1)
	if err == nil {
		t.Fatal("runDisplayScript() expected error for missing script")
	}
	if !strings.Contains(err.Error(), "start display script") {
		t.Fatalf("runDisplayScript() error = %q, want wrapped start error", err.Error())
	}
}

func TestRunDisplayScript_ReturnsContextCanceledOnCancel(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "display.sh")
	imagePath := filepath.Join(dir, "image.png")
	startedPath := filepath.Join(dir, "started")

	writeExecutableScript(t, scriptPath, "#!/bin/sh\ntrap 'exit 0' TERM INT\necho started > \""+startedPath+"\"\nwhile true; do sleep 1; done\n")
	if err := os.WriteFile(imagePath, []byte("img"), 0o644); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", imagePath, err)
	}

	srv := &imageServiceServer{displayScript: scriptPath}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan error, 1)
	go func() {
		resultCh <- srv.runDisplayScript(ctx, imagePath, 1)
	}()

	waitForCondition(t, time.Second, func() bool {
		_, err := os.Stat(startedPath)
		return err == nil
	})
	cancel()

	select {
	case err := <-resultCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("runDisplayScript() error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runDisplayScript() did not return after context cancellation")
	}
}

func TestShouldSuppressStreamReadError(t *testing.T) {
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "suppresses fs err closed after cancel", ctx: canceledCtx, err: fs.ErrClosed, want: true},
		{name: "suppresses file already closed after cancel", ctx: canceledCtx, err: errors.New("read 0: file already closed"), want: true},
		{name: "suppresses use of closed file after cancel", ctx: canceledCtx, err: errors.New("use of closed file"), want: true},
		{name: "does not suppress without cancel", ctx: context.Background(), err: fs.ErrClosed, want: false},
		{name: "does not suppress unrelated error", ctx: canceledCtx, err: errors.New("broken pipe"), want: false},
		{name: "does not suppress nil error", ctx: canceledCtx, err: nil, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldSuppressStreamReadError(tc.ctx, tc.err); got != tc.want {
				t.Fatalf("shouldSuppressStreamReadError(...) = %v, want %v", got, tc.want)
			}
		})
	}
}

type capturedLog struct {
	level slog.Level
	msg   string
	args  []any
}

func containsArgPair(args []any, key string, want any) bool {
	for i := 0; i+1 < len(args); i += 2 {
		if k, ok := args[i].(string); ok && k == key && args[i+1] == want {
			return true
		}
	}
	return false
}

func writeExecutableScript(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", path, err)
	}
}
