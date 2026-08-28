//go:build integration

package worker_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/worker"
)

func newRedisForQueue(t *testing.T) *redis.Client {
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

func TestRedisQueue_EnqueueDequeueRoundTrip(t *testing.T) {
	client := newRedisForQueue(t)
	q := worker.NewRedisQueue(client)
	ctx := context.Background()

	job := worker.Job{
		Type:    "token.cleanup",
		Payload: json.RawMessage(`{"foo":"bar"}`),
	}
	require.NoError(t, q.Enqueue(ctx, job))

	got, err := q.Dequeue(ctx, 2*time.Second)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotEmpty(t, got.ID, "Enqueue should assign an ID when none is set")
	require.Equal(t, job.Type, got.Type)
	require.JSONEq(t, string(job.Payload), string(got.Payload))
}

func TestRedisQueue_EnqueuePreservesExplicitID(t *testing.T) {
	client := newRedisForQueue(t)
	q := worker.NewRedisQueue(client)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, worker.Job{ID: "fixed-id", Type: "noop"}))

	got, err := q.Dequeue(ctx, 2*time.Second)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "fixed-id", got.ID)
}

func TestRedisQueue_DequeueTimesOutWhenEmpty(t *testing.T) {
	client := newRedisForQueue(t)
	q := worker.NewRedisQueue(client)
	ctx := context.Background()

	start := time.Now()
	got, err := q.Dequeue(ctx, 500*time.Millisecond)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Nil(t, got)
	require.GreaterOrEqual(t, elapsed, 400*time.Millisecond)
}

func TestRedisQueue_FIFOAcrossMultipleJobs(t *testing.T) {
	client := newRedisForQueue(t)
	q := worker.NewRedisQueue(client)
	ctx := context.Background()

	require.NoError(t, q.Enqueue(ctx, worker.Job{ID: "first", Type: "noop"}))
	require.NoError(t, q.Enqueue(ctx, worker.Job{ID: "second", Type: "noop"}))

	first, err := q.Dequeue(ctx, 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, "first", first.ID)

	second, err := q.Dequeue(ctx, 2*time.Second)
	require.NoError(t, err)
	require.Equal(t, "second", second.ID)
}
