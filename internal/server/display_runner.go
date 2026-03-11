package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const gracefulStopTimeout = 2 * time.Second

var emitLog = func(ctx context.Context, level slog.Level, msg string, args ...any) {
	slog.Log(ctx, level, msg, args...)
}

func (s *imageServiceServer) runDisplayScript(ctx context.Context, tempFilePath string, duration int32) error {
	cmd := exec.CommandContext(ctx, s.displayScript, tempFilePath, strconv.Itoa(int(duration)))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	slog.Info("executing display script", "script", s.displayScript, "args", cmd.Args[1:])
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start display script: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		var wg sync.WaitGroup
		wg.Add(2)
		// Drain both streams concurrently so the child process cannot block on full pipes.
		go streamScriptOutput(ctx, &wg, stdout, "stdout", tempFilePath)
		go streamScriptOutput(ctx, &wg, stderr, "stderr", tempFilePath)
		waitErr := cmd.Wait()
		wg.Wait()
		done <- waitErr
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("run display script: %w", err)
		}
		return nil
	case <-ctx.Done():
		if cmd.Process != nil {
			// Try graceful termination first, then SIGKILL if needed.
			slog.Warn("context canceled, terminating display script process", "pid", cmd.Process.Pid)
			if termErr := cmd.Process.Signal(syscall.SIGTERM); termErr != nil {
				slog.Warn("failed to send SIGTERM, forcing kill", "pid", cmd.Process.Pid, "error", termErr)
				_ = cmd.Process.Kill()
			} else {
				select {
				case <-done:
					return ctx.Err()
				case <-time.After(gracefulStopTimeout):
					slog.Warn("display script did not exit after SIGTERM, forcing kill", "pid", cmd.Process.Pid)
					_ = cmd.Process.Kill()
				}
			}

			// Wait briefly so the cmd.Wait goroutine can complete after cancellation.
			select {
			case <-done:
			case <-time.After(gracefulStopTimeout):
			}
		}
		return ctx.Err()
	}
}

func streamScriptOutput(ctx context.Context, wg *sync.WaitGroup, reader io.Reader, stream string, tempFilePath string) {
	defer wg.Done()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		logScriptLine(stream, scanner.Text(), tempFilePath)
	}
	if err := scanner.Err(); err != nil {
		if shouldSuppressStreamReadError(ctx, err) {
			return
		}
		slog.Warn("failed reading display script output", "stream", stream, "path", tempFilePath, "error", err)
	}
}

func shouldSuppressStreamReadError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() == nil {
		return false
	}
	if errors.Is(err, fs.ErrClosed) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "file already closed") || strings.Contains(message, "use of closed file")
}

func logScriptLine(stream string, line string, tempFilePath string) {
	attrs := []any{"source", "display-script", "stream", stream, "path", tempFilePath, "line", line}
	// Preserve script severity hints when present; otherwise infer from stream.
	if strings.Contains(line, "[display-script][ERROR]") {
		emitLog(context.Background(), slog.LevelError, "display script output", attrs...)
		return
	}
	if strings.Contains(line, "[display-script][WARN]") || stream == "stderr" {
		emitLog(context.Background(), slog.LevelWarn, "display script output", attrs...)
		return
	}
	emitLog(context.Background(), slog.LevelInfo, "display script output", attrs...)
}
