package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveWorkerScriptTimeout_DefaultWhenUnset(t *testing.T) {
	t.Setenv(envWorkerScriptTimeout, "")

	got, err := resolveWorkerScriptTimeout()
	if err != nil {
		t.Fatalf("resolveWorkerScriptTimeout() error = %v", err)
	}
	if got != defaultWorkerTimeout {
		t.Fatalf("resolveWorkerScriptTimeout() = %v, want %v", got, defaultWorkerTimeout)
	}
}

func TestResolveWorkerScriptTimeout_ParsesValidDuration(t *testing.T) {
	t.Setenv(envWorkerScriptTimeout, "45s")

	got, err := resolveWorkerScriptTimeout()
	if err != nil {
		t.Fatalf("resolveWorkerScriptTimeout() error = %v", err)
	}
	if got != 45*time.Second {
		t.Fatalf("resolveWorkerScriptTimeout() = %v, want %v", got, 45*time.Second)
	}
}

func TestResolveWorkerScriptTimeout_FailsOnInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "invalid format", value: "abc"},
		{name: "zero", value: "0s"},
		{name: "negative", value: "-1s"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envWorkerScriptTimeout, tc.value)
			if _, err := resolveWorkerScriptTimeout(); err == nil {
				t.Fatalf("resolveWorkerScriptTimeout() expected error for %q", tc.value)
			}
		})
	}
}

func TestResolveConfig_FailsOnInvalidWorkerScriptTimeout(t *testing.T) {
	scriptPath := createTempScript(t)
	t.Setenv(envDisplayScript, scriptPath)
	t.Setenv(envWorkerScriptTimeout, "bad")

	if _, err := resolveConfig(); err == nil {
		t.Fatal("resolveConfig() expected error for invalid worker timeout")
	}
}

func TestResolveConfig_UsesDefaultWorkerTimeoutWhenUnset(t *testing.T) {
	scriptPath := createTempScript(t)
	t.Setenv(envDisplayScript, scriptPath)
	t.Setenv(envWorkerScriptTimeout, "")

	cfg, err := resolveConfig()
	if err != nil {
		t.Fatalf("resolveConfig() error = %v", err)
	}
	defer func() {
		_ = os.RemoveAll(cfg.tempDir)
	}()

	if cfg.workerScriptTimeout != defaultWorkerTimeout {
		t.Fatalf("cfg.workerScriptTimeout = %v, want %v", cfg.workerScriptTimeout, defaultWorkerTimeout)
	}
}

func createTempScript(t *testing.T) string {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "display_image.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", scriptPath, err)
	}
	return scriptPath
}
