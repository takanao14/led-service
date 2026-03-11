package server

import (
	"fmt"
	"log/slog"
	"os"
)

const (
	tempFilePrefix = "led-image-"
	tempFileMode   = 0o644
)

func removeTempFile(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		slog.Warn("failed to remove temp file", "path", path, "error", err)
	}
}

func (s *imageServiceServer) saveTempImage(imageData []byte, mimeType string) (string, error) {
	ext := determineExtension(mimeType)

	tempFile, err := os.CreateTemp(s.tempDir, tempFilePrefix+"*"+ext)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tempFilePath := tempFile.Name()

	if _, err := tempFile.Write(imageData); err != nil {
		tempFile.Close()
		os.Remove(tempFilePath)
		return "", fmt.Errorf("write temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempFilePath)
		return "", fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tempFilePath, tempFileMode); err != nil {
		os.Remove(tempFilePath)
		return "", fmt.Errorf("chmod temp file: %w", err)
	}

	return tempFilePath, nil
}

func determineExtension(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/x-portable-pixmap":
		return ".ppm"
	}
	return ".dat"
}
