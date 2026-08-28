package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

type Handler func(ctx context.Context, payload json.RawMessage) error

type Worker struct {
	queue    Queue
	handlers map[string]Handler
}

func NewWorker(queue Queue) *Worker {
	return &Worker{queue: queue, handlers: make(map[string]Handler)}
}

func (w *Worker) Register(jobType string, handler Handler) {
	w.handlers[jobType] = handler
}

func (w *Worker) Run(ctx context.Context) {
	slog.Info("worker started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("worker stopped")
			return
		default:
			job, err := w.queue.Dequeue(ctx, 5*time.Second)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Error("dequeue error", "error", err)
				continue
			}
			if job == nil {
				continue
			}

			handler, ok := w.handlers[job.Type]
			if !ok {
				slog.Error("unknown job type", "type", job.Type)
				continue
			}

			if err := handler(ctx, job.Payload); err != nil {
				slog.Error("job failed", "type", job.Type, "id", job.ID, "retries", job.Retries, "error", err)
				if job.Retries < 5 {
					job.Retries++
					_ = w.queue.Enqueue(ctx, *job)
				}
			} else {
				slog.Info("job completed", "type", job.Type, "id", job.ID)
			}
		}
	}
}
