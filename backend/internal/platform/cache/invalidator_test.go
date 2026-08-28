package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/cache"
)

type fakeCache struct {
	deleted []string
}

func (f *fakeCache) Get(_ context.Context, _ string, _ interface{}) error { return cache.ErrCacheMiss }
func (f *fakeCache) Set(_ context.Context, _ string, _ interface{}, _ time.Duration) error {
	return nil
}
func (f *fakeCache) Delete(_ context.Context, keys ...string) error {
	f.deleted = append(f.deleted, keys...)
	return nil
}

type fakePublisher struct {
	published []string
}

func (f *fakePublisher) Publish(_ context.Context, _ uuid.UUID, eventType string, _ interface{}) {
	f.published = append(f.published, eventType)
}

func TestCacheInvalidator_DeletesAndDelegates(t *testing.T) {
	fc := &fakeCache{}
	fp := &fakePublisher{}
	inv := cache.NewInvalidator(fc, fp)

	boardID := uuid.New()
	inv.Publish(context.Background(), boardID, "card.created", nil)

	require.Len(t, fc.deleted, 1)
	require.Contains(t, fc.deleted[0], boardID.String())
	require.Equal(t, []string{"card.created"}, fp.published)
}
