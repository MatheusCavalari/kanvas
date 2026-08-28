//go:build integration

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/cache"
)

func newRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "redis:7-alpine",
			ExposedPorts: []string{"6379/tcp"},
			WaitingFor:   wait.ForLog("Ready to accept connections"),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379")
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{Addr: host + ":" + port.Port()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestRedisCache_SetGetDelete(t *testing.T) {
	client := newRedisClient(t)
	c := cache.NewRedisCache(client)
	ctx := context.Background()

	type testData struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	err := c.Set(ctx, "test:1", testData{Name: "board", Count: 5}, 10*time.Second)
	require.NoError(t, err)

	var got testData
	err = c.Get(ctx, "test:1", &got)
	require.NoError(t, err)
	require.Equal(t, "board", got.Name)
	require.Equal(t, 5, got.Count)

	err = c.Delete(ctx, "test:1")
	require.NoError(t, err)

	err = c.Get(ctx, "test:1", &got)
	require.ErrorIs(t, err, cache.ErrCacheMiss)
}
