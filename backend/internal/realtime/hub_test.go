package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHub_PublishDeliversToSubscriber(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	ctx := context.Background()

	ch := hub.subscribe(boardID)
	defer hub.unsubscribe(boardID, ch)
	require.Equal(t, 1, hub.SubscriberCount(boardID))

	hub.Publish(ctx, boardID, "card.created", map[string]string{"id": "abc"})

	select {
	case event := <-ch:
		require.Equal(t, "card.created", event.Type)
		require.Equal(t, boardID, event.BoardID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestHub_PublishOnlyReachesSubscribersOfThatBoard(t *testing.T) {
	hub := NewHub()
	boardA := uuid.New()
	boardB := uuid.New()
	ctx := context.Background()

	chA := hub.subscribe(boardA)
	defer hub.unsubscribe(boardA, chA)
	chB := hub.subscribe(boardB)
	defer hub.unsubscribe(boardB, chB)

	hub.Publish(ctx, boardA, "card.created", nil)

	select {
	case <-chA:
	case <-time.After(time.Second):
		t.Fatal("boardA subscriber should have received the event")
	}

	select {
	case <-chB:
		t.Fatal("boardB subscriber should not have received boardA's event")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHub_UnsubscribeStopsDelivery(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	ctx := context.Background()

	ch := hub.subscribe(boardID)
	hub.unsubscribe(boardID, ch)
	require.Equal(t, 0, hub.SubscriberCount(boardID))

	hub.Publish(ctx, boardID, "card.created", nil)

	_, ok := <-ch
	require.False(t, ok, "channel should be closed after unsubscribe")
}

func TestHub_PublishDoesNotBlockOnFullSubscriberBuffer(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	ctx := context.Background()

	ch := hub.subscribe(boardID)
	defer hub.unsubscribe(boardID, ch)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			hub.Publish(ctx, boardID, "card.created", i)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish should never block, even with an unread subscriber buffer")
	}
}
