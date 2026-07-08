// Package local provides a local filesystem storage adapter.
package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Adapter implements the knowledge.Storage interface using the local filesystem.
type Adapter struct {
	basePath string
}

// New creates a new local filesystem storage adapter.
func New(basePath string) (*Adapter, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	return &Adapter{basePath: basePath}, nil
}

func (a *Adapter) Upload(_ context.Context, _, key string, reader io.Reader, _ int64, _ string) (string, error) {
	path := filepath.Join(a.basePath, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, reader); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	return key, nil
}

func (a *Adapter) Download(_ context.Context, _, key string) (io.ReadCloser, error) {
	path := filepath.Join(a.basePath, key)
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return f, nil
}

func (a *Adapter) Delete(_ context.Context, _, key string) error {
	path := filepath.Join(a.basePath, key)
	return os.Remove(path)
}

func (a *Adapter) GetPresignedURL(_ context.Context, _, key string, _ time.Duration) (string, error) {
	// Local storage doesn't support presigned URLs; return the key as-is
	return key, nil
}
