// Package redis provides a Redis cache adapter.
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Adapter implements the knowledge.Cache interface for Redis.
type Adapter struct {
	client *goredis.Client
}

// New creates a new Redis cache adapter.
func New(redisURL string) (*Adapter, error) {
	opts, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}
	client := goredis.NewClient(opts)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}
	return &Adapter{client: client}, nil
}

func (a *Adapter) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := a.client.Get(ctx, key).Bytes()
	if err == goredis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	return val, nil
}

func (a *Adapter) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return a.client.Set(ctx, key, value, ttl).Err()
}

func (a *Adapter) Delete(ctx context.Context, keys ...string) error {
	return a.client.Del(ctx, keys...).Err()
}

func (a *Adapter) Close() error {
	return a.client.Close()
}
