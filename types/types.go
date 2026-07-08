// Package types contains shared types used across the Knowledge Engine.
// This package has no internal dependencies and can be imported by any sub-package.
package types

import (
	"context"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UserInfo holds the authenticated user's identity.
type UserInfo struct {
	ID    string
	Email string
	Role  string
	OrgID string
}

// Database abstracts the database connection.
type Database interface {
	Pool() *pgxpool.Pool
	Close() error
	Ping(ctx context.Context) error
}

// Cache abstracts a key-value cache.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}

// Storage abstracts object storage.
type Storage interface {
	Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) (string, error)
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, bucket, key string) error
	GetPresignedURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}

// Authenticator abstracts authentication.
type Authenticator interface {
	Authenticate(token string) (*UserInfo, error)
}

// Logger abstracts structured logging.
type Logger interface {
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
	Debug(msg string, keysAndValues ...any)
}
