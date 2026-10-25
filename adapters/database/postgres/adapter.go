// Package postgres provides a PostgreSQL adapter for the Knowledge Engine.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Adapter implements the knowledge.Database interface for PostgreSQL.
type Adapter struct {
	pool *pgxpool.Pool
}

// New creates a new PostgreSQL adapter.
func New(databaseURL string) (*Adapter, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	return &Adapter{pool: pool}, nil
}

func (a *Adapter) Pool() *pgxpool.Pool            { return a.pool }
func (a *Adapter) Close() error                   { a.pool.Close(); return nil }
func (a *Adapter) Ping(ctx context.Context) error { return a.pool.Ping(ctx) }
