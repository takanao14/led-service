package server

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	defaultListenAddr      = ":50051"
	defaultDisplayScript   = "scripts/display_image.sh"
	defaultWorkerTimeout   = 30 * time.Second
	envGrpcAddr            = "GRPC_ADDR"
	envDisplayScript       = "DISPLAY_SCRIPT"
	envWorkerScriptTimeout = "WORKER_SCRIPT_TIMEOUT"
	tempDirMode            = 0o755
)

type runtimeConfig struct {
	listenAddr          string
	displayScript       string
	tempDir             string
	workerScriptTimeout time.Duration
}

func resolveConfig() (*runtimeConfig, error) {
	listenAddr := getEnv(envGrpcAddr, defaultListenAddr)
	displayScriptPath := getEnv(envDisplayScript, "")
	workerScriptTimeout, err := resolveWorkerScriptTimeout()
	if err != nil {
		return nil, err
	}
	slog.Debug("resolved startup config", "addr", listenAddr, "display_script", displayScriptPath, "worker_script_timeout", workerScriptTimeout)

	if displayScriptPath == "" {
		// If DISPLAY_SCRIPT is not set, try to find it in the working directory
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		displayScriptPath = filepath.Join(cwd, defaultDisplayScript)

		// If it doesn't exist in CWD/scripts, try common install location /opt/led-service/scripts/
		if _, err := os.Stat(displayScriptPath); err != nil {
			const commonInstallScript = "/opt/led-service/scripts/display_image.sh"
			if _, statErr := os.Stat(commonInstallScript); statErr == nil {
				displayScriptPath = commonInstallScript
			}
		}
	}

	if _, err := os.Stat(displayScriptPath); err != nil {
		return nil, fmt.Errorf("display script not found: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "grpc-image-server-*")
	if err != nil {
		return nil, fmt.Errorf("create temp directory: %w", err)
	}
	if err := os.Chmod(tempDir, tempDirMode); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("set temp directory permission: %w", err)
	}

	return &runtimeConfig{
		listenAddr:          listenAddr,
		displayScript:       displayScriptPath,
		tempDir:             tempDir,
		workerScriptTimeout: workerScriptTimeout,
	}, nil
}

func resolveWorkerScriptTimeout() (time.Duration, error) {
	raw := os.Getenv(envWorkerScriptTimeout)
	if raw == "" {
		return defaultWorkerTimeout, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", envWorkerScriptTimeout, raw, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("invalid %s %q: must be greater than 0", envWorkerScriptTimeout, raw)
	}
	return parsed, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
