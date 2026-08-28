package webhook

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/worker"
)

// deliverJobType is the worker.Queue job type handled by the deliverer
// returned from NewDeliverer.
const deliverJobType = "webhook.deliver"

// deliverJobPayload is the JSON body of a "webhook.deliver" job. It
// references a Delivery row (created up front by the Dispatcher) rather
// than embedding the event data twice, so the deliverer's attempts all
// update the same delivery record.
type deliverJobPayload struct {
	DeliveryID string          `json:"delivery_id"`
	WebhookID  string          `json:"webhook_id"`
	EventType  string          `json:"event_type"`
	Data       json.RawMessage `json:"data"`
}

// Dispatcher is an EventPublisher decorator that, for every event, enqueues
// a delivery job for each active webhook on the board subscribed to that
// event type, before delegating to the next publisher in the chain.
type Dispatcher struct {
	repo  Repository
	queue worker.Queue
	next  EventPublisher
}

func NewDispatcher(repo Repository, queue worker.Queue, next EventPublisher) *Dispatcher {
	return &Dispatcher{repo: repo, queue: queue, next: next}
}

func (d *Dispatcher) Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
	d.dispatch(ctx, boardID, eventType, payload)
	d.next.Publish(ctx, boardID, eventType, payload)
}

// dispatch looks up the board's active webhooks subscribed to eventType and
// enqueues a delivery job for each. Any failure (lookup, marshaling,
// persisting the delivery row, or enqueueing) is logged and otherwise
// swallowed: webhook delivery is best-effort and must never block or fail
// the event publish itself.
func (d *Dispatcher) dispatch(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
	webhooks, err := d.repo.ListActiveByBoardAndEvent(ctx, boardID, eventType)
	if err != nil {
		slog.Warn("webhook: failed to list active webhooks", "board_id", boardID, "event_type", eventType, "error", err)
		return
	}
	if len(webhooks) == 0 {
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("webhook: failed to marshal event payload", "board_id", boardID, "event_type", eventType, "error", err)
		return
	}

	for _, wh := range webhooks {
		delivery, err := d.repo.CreateDelivery(ctx, Delivery{
			ID:        uuid.New(),
			WebhookID: wh.ID,
			EventType: eventType,
			Payload:   data,
			Status:    DeliveryStatusPending,
		})
		if err != nil {
			slog.Warn("webhook: failed to create delivery record", "webhook_id", wh.ID, "error", err)
			continue
		}

		jobPayload, err := json.Marshal(deliverJobPayload{
			DeliveryID: delivery.ID.String(),
			WebhookID:  wh.ID.String(),
			EventType:  eventType,
			Data:       data,
		})
		if err != nil {
			slog.Warn("webhook: failed to marshal delivery job payload", "webhook_id", wh.ID, "delivery_id", delivery.ID, "error", err)
			continue
		}

		if err := d.queue.Enqueue(ctx, worker.Job{Type: deliverJobType, Payload: jobPayload}); err != nil {
			slog.Warn("webhook: failed to enqueue delivery job", "webhook_id", wh.ID, "delivery_id", delivery.ID, "error", err)
		}
	}
}
