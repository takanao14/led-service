package server

import (
	"log/slog"
	"os"
)

const envDebug = "DEBUG"

func configureLogger() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	if os.Getenv(envDebug) != "" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug, AddSource: true})
	}
	logger := slog.New(handler).With("service", "led-server")
	slog.SetDefault(logger)
	slog.Debug("debug logging enabled", "env", envDebug)
}
