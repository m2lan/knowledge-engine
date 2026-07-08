// Package memory provides an in-memory cache adapter.
package memory

import (
	"context"
	"sync"
	"time"
)

type entry struct {
	value     []byte
	expiresAt time.Time
}

// Adapter implements the knowledge.Cache interface using an in-memory map.
type Adapter struct {
	mu      sync.RWMutex
	items   map[string]entry
	closeCh chan struct{}
}

// New creates a new in-memory cache adapter.
func New() *Adapter {
	a := &Adapter{
		items:   make(map[string]entry),
		closeCh: make(chan struct{}),
	}
	go a.cleanup()
	return a
}

func (a *Adapter) Get(_ context.Context, key string) ([]byte, error) {
	a.mu.RLock()
	e, ok := a.items[key]
	a.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	if !e.expiresAt.IsZero() && time.Now().After(e.expiresAt) {
		a.mu.Lock()
		delete(a.items, key)
		a.mu.Unlock()
		return nil, nil
	}
	return e.value, nil
}

func (a *Adapter) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	a.mu.Lock()
	a.items[key] = entry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	a.mu.Unlock()
	return nil
}

func (a *Adapter) Delete(_ context.Context, keys ...string) error {
	a.mu.Lock()
	for _, key := range keys {
		delete(a.items, key)
	}
	a.mu.Unlock()
	return nil
}

func (a *Adapter) Close() error {
	close(a.closeCh)
	return nil
}

func (a *Adapter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			a.mu.Lock()
			for k, e := range a.items {
				if !e.expiresAt.IsZero() && now.After(e.expiresAt) {
					delete(a.items, k)
				}
			}
			a.mu.Unlock()
		case <-a.closeCh:
			return
		}
	}
}
