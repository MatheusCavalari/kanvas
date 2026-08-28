package cache

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// EventPublisher is the local, decoupled view of the publisher this package
// wraps. It intentionally mirrors internal/card/repository.go's
// EventPublisher interface rather than importing it, so this package does
// not depend on the card package.
type EventPublisher interface {
	Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{})
}

// CacheInvalidator is an EventPublisher decorator that invalidates the
// cached board data before delegating the event to the next publisher in
// the chain (e.g. the realtime hub).
type CacheInvalidator struct {
	cache Cache
	next  EventPublisher
}

func NewInvalidator(c Cache, next EventPublisher) *CacheInvalidator {
	return &CacheInvalidator{cache: c, next: next}
}

func (ci *CacheInvalidator) Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
	key := fmt.Sprintf("board:%s:data", boardID)
	if err := ci.cache.Delete(ctx, key); err != nil {
		slog.Warn("cache invalidation failed", "board_id", boardID, "error", err)
	}
	ci.next.Publish(ctx, boardID, eventType, payload)
}
