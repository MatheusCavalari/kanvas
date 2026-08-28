package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/worker"
)

// fakeQueue is an in-memory implementation of worker.Queue used to drive
// the Worker in unit tests without a real Redis instance.
type fakeQueue struct {
	mu   sync.Mutex
	jobs []worker.Job
	// dequeued records every job handed out by Dequeue, in order.
	dequeued []worker.Job
}

func (q *fakeQueue) Enqueue(_ context.Context, job worker.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if job.ID == "" {
		job.ID = "generated"
	}
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *fakeQueue) Dequeue(ctx context.Context, timeout time.Duration) (*worker.Job, error) {
	deadline := time.Now().Add(timeout)
	for {
		q.mu.Lock()
		if len(q.jobs) > 0 {
			job := q.jobs[0]
			q.jobs = q.jobs[1:]
			q.dequeued = append(q.dequeued, job)
			q.mu.Unlock()
			return &job, nil
		}
		q.mu.Unlock()

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if time.Now().After(deadline) {
			return nil, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
}

func (q *fakeQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs)
}

func TestWorker_DispatchesToRegisteredHandler(t *testing.T) {
	q := &fakeQueue{}
	w := worker.NewWorker(q)

	var mu sync.Mutex
	var received json.RawMessage
	done := make(chan struct{})

	w.Register("greet", func(_ context.Context, payload json.RawMessage) error {
		mu.Lock()
		received = payload
		mu.Unlock()
		close(done)
		return nil
	})

	require.NoError(t, q.Enqueue(context.Background(), worker.Job{
		Type:    "greet",
		Payload: json.RawMessage(`{"name":"kanvas"}`),
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not invoked in time")
	}

	mu.Lock()
	defer mu.Unlock()
	require.JSONEq(t, `{"name":"kanvas"}`, string(received))
}

func TestWorker_RetriesFailedJobsUpToLimit(t *testing.T) {
	q := &fakeQueue{}
	w := worker.NewWorker(q)

	var attempts int
	var mu sync.Mutex

	w.Register("fail", func(_ context.Context, _ json.RawMessage) error {
		mu.Lock()
		attempts++
		mu.Unlock()
		return errors.New("boom")
	})

	require.NoError(t, q.Enqueue(context.Background(), worker.Job{
		Type:    "fail",
		Retries: 5, // already at the retry ceiling; the next failure must not re-enqueue
	}))

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return attempts == 1
	}, 2*time.Second, 10*time.Millisecond)

	// Give the worker a moment to (not) re-enqueue.
	time.Sleep(50 * time.Millisecond)
	cancel()

	require.Equal(t, 0, q.len(), "job at the retry ceiling should not be re-enqueued")
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, attempts)
}

func TestWorker_ReEnqueuesFailedJobBelowRetryLimit(t *testing.T) {
	q := &fakeQueue{}
	w := worker.NewWorker(q)

	w.Register("fail-once", func(_ context.Context, _ json.RawMessage) error {
		return errors.New("transient")
	})

	require.NoError(t, q.Enqueue(context.Background(), worker.Job{Type: "fail-once"}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	require.Eventually(t, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		for _, j := range q.dequeued {
			if j.Retries >= 1 {
				return true
			}
		}
		return len(q.dequeued) >= 2
	}, 2*time.Second, 10*time.Millisecond)
}

func TestWorker_UnknownJobTypeIsSkippedNotRetried(t *testing.T) {
	q := &fakeQueue{}
	w := worker.NewWorker(q)

	require.NoError(t, q.Enqueue(context.Background(), worker.Job{Type: "mystery"}))

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)

	require.Eventually(t, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		return len(q.dequeued) == 1
	}, 2*time.Second, 10*time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	cancel()

	require.Equal(t, 0, q.len(), "unknown job type must not be re-enqueued")
}

func TestWorker_StopsWhenContextCancelled(t *testing.T) {
	q := &fakeQueue{}
	w := worker.NewWorker(q)

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(stopped)
	}()

	cancel()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}
