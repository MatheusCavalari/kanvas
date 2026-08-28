package cache

import (
	"context"
	"time"
)

// Cache is a simple key-value cache abstraction with JSON serialization
// semantics for Set/Get and best-effort deletion.
type Cache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}
