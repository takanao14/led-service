package main

import (
	"log/slog"
	"os"

	"led-service/internal/server"
)

func main() {
	if err := server.Run(); err != nil {
		slog.Error("led-server exited with error", "error", err)
		os.Exit(1)
	}
}
