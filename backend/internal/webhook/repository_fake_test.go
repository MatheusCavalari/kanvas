package webhook_test

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/worker"
	"github.com/MatheusCavalari/kanvas/backend/internal/webhook"
)

type fakeRepo struct {
	mu         sync.Mutex
	webhooks   map[uuid.UUID]webhook.Webhook
	deliveries map[uuid.UUID]webhook.Delivery
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		webhooks:   map[uuid.UUID]webhook.Webhook{},
		deliveries: map[uuid.UUID]webhook.Delivery{},
	}
}

func (f *fakeRepo) Create(_ context.Context, w webhook.Webhook) (webhook.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now()
	}
	f.webhooks[w.ID] = w
	return w, nil
}

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (webhook.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.webhooks[id]
	if !ok {
		return webhook.Webhook{}, webhook.ErrNotFound
	}
	return w, nil
}

func (f *fakeRepo) Update(_ context.Context, id uuid.UUID, url string, events []string, active bool) (webhook.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.webhooks[id]
	if !ok {
		return webhook.Webhook{}, webhook.ErrNotFound
	}
	w.URL = url
	w.Events = events
	w.Active = active
	f.webhooks[id] = w
	return w, nil
}

func (f *fakeRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.webhooks, id)
	return nil
}

func (f *fakeRepo) ListByBoard(_ context.Context, boardID uuid.UUID) ([]webhook.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []webhook.Webhook
	for _, w := range f.webhooks {
		if w.BoardID == boardID {
			result = append(result, w)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (f *fakeRepo) ListActiveByBoardAndEvent(_ context.Context, boardID uuid.UUID, eventType string) ([]webhook.Webhook, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []webhook.Webhook
	for _, w := range f.webhooks {
		if w.BoardID != boardID || !w.Active {
			continue
		}
		for _, e := range w.Events {
			if e == eventType {
				result = append(result, w)
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (f *fakeRepo) CreateDelivery(_ context.Context, d webhook.Delivery) (webhook.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	f.deliveries[d.ID] = d
	return d, nil
}

func (f *fakeRepo) UpdateDeliveryStatus(_ context.Context, id uuid.UUID, status string, responseCode *int, lastAttemptAt time.Time) (webhook.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.deliveries[id]
	if !ok {
		return webhook.Delivery{}, webhook.ErrNotFound
	}
	d.Status = status
	d.Attempts++
	d.ResponseCode = responseCode
	lat := lastAttemptAt
	d.LastAttemptAt = &lat
	f.deliveries[id] = d
	return d, nil
}

func (f *fakeRepo) ListDeliveriesByWebhook(_ context.Context, webhookID uuid.UUID, cursor time.Time, limit int) ([]webhook.Delivery, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []webhook.Delivery
	for _, d := range f.deliveries {
		if d.WebhookID == webhookID && d.CreatedAt.Before(cursor) {
			result = append(result, d)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

type fakeOwnerChecker struct {
	mu     sync.Mutex
	owners map[uuid.UUID]bool
}

func newFakeOwnerChecker() *fakeOwnerChecker {
	return &fakeOwnerChecker{owners: map[uuid.UUID]bool{}}
}

func (f *fakeOwnerChecker) IsBoardOwner(_ context.Context, _, userID uuid.UUID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.owners[userID]
}

type fakeQueue struct {
	mu   sync.Mutex
	jobs []worker.Job
}

func newFakeQueue() *fakeQueue {
	return &fakeQueue{}
}

func (f *fakeQueue) Enqueue(_ context.Context, job worker.Job) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobs = append(f.jobs, job)
	return nil
}

func (f *fakeQueue) Dequeue(_ context.Context, _ time.Duration) (*worker.Job, error) {
	return nil, nil
}

func (f *fakeQueue) all() []worker.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]worker.Job, len(f.jobs))
	copy(out, f.jobs)
	return out
}

type fakeEventPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	boardID   uuid.UUID
	eventType string
	payload   interface{}
}

func (f *fakeEventPublisher) Publish(_ context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, publishedEvent{boardID: boardID, eventType: eventType, payload: payload})
}

func (f *fakeEventPublisher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.events)
}

// farFuture is used as the cursor for a DESC listing's first page, mirroring
// the production handler's farFutureCursor.
func farFuture() time.Time {
	return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
}
