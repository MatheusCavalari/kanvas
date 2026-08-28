package webhook_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/webhook"
)

func TestDispatcher_EnqueuesJobForMatchingWebhook(t *testing.T) {
	repo := newFakeRepo()
	queue := newFakeQueue()
	next := &fakeEventPublisher{}
	dispatcher := webhook.NewDispatcher(repo, queue, next)
	ctx := context.Background()

	boardID := uuid.New()
	wh, err := repo.Create(ctx, webhook.Webhook{
		ID:      uuid.New(),
		BoardID: boardID,
		OwnerID: uuid.New(),
		URL:     "https://example.com/hook",
		Secret:  "s3cr3t",
		Events:  []string{"card.created"},
		Active:  true,
	})
	require.NoError(t, err)

	dispatcher.Publish(ctx, boardID, "card.created", map[string]string{"id": "abc"})

	jobs := queue.all()
	require.Len(t, jobs, 1)
	require.Equal(t, "webhook.deliver", jobs[0].Type)

	var payload struct {
		DeliveryID string          `json:"delivery_id"`
		WebhookID  string          `json:"webhook_id"`
		EventType  string          `json:"event_type"`
		Data       json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(jobs[0].Payload, &payload))
	require.Equal(t, wh.ID.String(), payload.WebhookID)
	require.Equal(t, "card.created", payload.EventType)
	require.NotEmpty(t, payload.DeliveryID)

	// A delivery row should have been created up front, in pending status.
	deliveries, err := repo.ListDeliveriesByWebhook(ctx, wh.ID, farFuture(), 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.Equal(t, webhook.DeliveryStatusPending, deliveries[0].Status)

	// The event is always delegated to next, regardless of any webhooks.
	require.Equal(t, 1, next.count())
}

func TestDispatcher_SkipsNonMatchingEventType(t *testing.T) {
	repo := newFakeRepo()
	queue := newFakeQueue()
	next := &fakeEventPublisher{}
	dispatcher := webhook.NewDispatcher(repo, queue, next)
	ctx := context.Background()

	boardID := uuid.New()
	_, err := repo.Create(ctx, webhook.Webhook{
		ID: uuid.New(), BoardID: boardID, OwnerID: uuid.New(),
		URL: "https://example.com/hook", Secret: "s3cr3t",
		Events: []string{"card.created"}, Active: true,
	})
	require.NoError(t, err)

	dispatcher.Publish(ctx, boardID, "comment.created", map[string]string{"id": "abc"})

	require.Empty(t, queue.all())
	require.Equal(t, 1, next.count())
}

func TestDispatcher_SkipsInactiveWebhook(t *testing.T) {
	repo := newFakeRepo()
	queue := newFakeQueue()
	next := &fakeEventPublisher{}
	dispatcher := webhook.NewDispatcher(repo, queue, next)
	ctx := context.Background()

	boardID := uuid.New()
	_, err := repo.Create(ctx, webhook.Webhook{
		ID: uuid.New(), BoardID: boardID, OwnerID: uuid.New(),
		URL: "https://example.com/hook", Secret: "s3cr3t",
		Events: []string{"card.created"}, Active: false,
	})
	require.NoError(t, err)

	dispatcher.Publish(ctx, boardID, "card.created", map[string]string{"id": "abc"})

	require.Empty(t, queue.all())
}

func TestDispatcher_SkipsOtherBoard(t *testing.T) {
	repo := newFakeRepo()
	queue := newFakeQueue()
	next := &fakeEventPublisher{}
	dispatcher := webhook.NewDispatcher(repo, queue, next)
	ctx := context.Background()

	boardID := uuid.New()
	otherBoardID := uuid.New()
	_, err := repo.Create(ctx, webhook.Webhook{
		ID: uuid.New(), BoardID: boardID, OwnerID: uuid.New(),
		URL: "https://example.com/hook", Secret: "s3cr3t",
		Events: []string{"card.created"}, Active: true,
	})
	require.NoError(t, err)

	dispatcher.Publish(ctx, otherBoardID, "card.created", map[string]string{"id": "abc"})

	require.Empty(t, queue.all())
}

func TestDispatcher_MultipleMatchingWebhooks(t *testing.T) {
	repo := newFakeRepo()
	queue := newFakeQueue()
	next := &fakeEventPublisher{}
	dispatcher := webhook.NewDispatcher(repo, queue, next)
	ctx := context.Background()

	boardID := uuid.New()
	for i := 0; i < 3; i++ {
		_, err := repo.Create(ctx, webhook.Webhook{
			ID: uuid.New(), BoardID: boardID, OwnerID: uuid.New(),
			URL: "https://example.com/hook", Secret: "s3cr3t",
			Events: []string{"card.created"}, Active: true,
		})
		require.NoError(t, err)
	}

	dispatcher.Publish(ctx, boardID, "card.created", map[string]string{"id": "abc"})

	require.Len(t, queue.all(), 3)
}
