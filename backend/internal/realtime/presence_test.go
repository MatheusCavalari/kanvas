package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHub_JoinPresence_AddsToMapAndBroadcasts(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	userID := uuid.New()
	ctx := context.Background()

	ch := hub.subscribe(boardID)
	defer hub.unsubscribe(boardID, ch)

	hub.JoinPresence(boardID, userID, "Ada Lovelace")

	list := hub.GetPresence(boardID)
	require.Len(t, list, 1)
	require.Equal(t, userID, list[0].UserID)
	require.Equal(t, "Ada Lovelace", list[0].Name)

	select {
	case event := <-ch:
		require.Equal(t, "presence.joined", event.Type)
		require.Equal(t, boardID, event.BoardID)
		info, ok := event.Data.(PresenceInfo)
		require.True(t, ok)
		require.Equal(t, userID, info.UserID)
		require.Equal(t, "Ada Lovelace", info.Name)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for presence.joined event")
	}

	_ = ctx
}

func TestHub_LeavePresence_RemovesFromMapAndBroadcasts(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	userID := uuid.New()

	hub.JoinPresence(boardID, userID, "Ada Lovelace")

	ch := hub.subscribe(boardID)
	defer hub.unsubscribe(boardID, ch)

	hub.LeavePresence(boardID, userID)

	require.Empty(t, hub.GetPresence(boardID))

	select {
	case event := <-ch:
		require.Equal(t, "presence.left", event.Type)
		require.Equal(t, boardID, event.BoardID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for presence.left event")
	}
}

func TestHub_LeavePresence_NoOpWhenNotPresent(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	userID := uuid.New()

	ch := hub.subscribe(boardID)
	defer hub.unsubscribe(boardID, ch)

	// Leaving a user who never joined must not broadcast anything.
	hub.LeavePresence(boardID, userID)

	select {
	case event := <-ch:
		t.Fatalf("unexpected event broadcast: %+v", event)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHub_GetPresence_ReturnsCurrentList(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	userA := uuid.New()
	userB := uuid.New()

	require.Empty(t, hub.GetPresence(boardID))

	hub.JoinPresence(boardID, userA, "Alice")
	hub.JoinPresence(boardID, userB, "Bob")

	list := hub.GetPresence(boardID)
	require.Len(t, list, 2)

	names := map[uuid.UUID]string{}
	for _, info := range list {
		names[info.UserID] = info.Name
	}
	require.Equal(t, "Alice", names[userA])
	require.Equal(t, "Bob", names[userB])

	hub.LeavePresence(boardID, userA)
	require.Len(t, hub.GetPresence(boardID), 1)
}

func TestHub_Heartbeat_UpdatesLastHeartbeat(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	userID := uuid.New()

	hub.JoinPresence(boardID, userID, "Ada")

	hub.mu.RLock()
	before := hub.presence[boardID][userID].LastHeartbeat
	hub.mu.RUnlock()

	time.Sleep(5 * time.Millisecond)
	hub.Heartbeat(boardID, userID)

	hub.mu.RLock()
	after := hub.presence[boardID][userID].LastHeartbeat
	hub.mu.RUnlock()

	require.True(t, after.After(before))
}

func TestHub_Heartbeat_NoOpWhenNotPresent(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	userID := uuid.New()

	// Must not panic or create a phantom presence entry.
	require.NotPanics(t, func() {
		hub.Heartbeat(boardID, userID)
	})
	require.Empty(t, hub.GetPresence(boardID))
}

func TestHub_Reaper_RemovesStaleEntries(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	staleUser := uuid.New()
	freshUser := uuid.New()

	hub.JoinPresence(boardID, staleUser, "Stale")
	hub.JoinPresence(boardID, freshUser, "Fresh")

	// Backdate staleUser's heartbeat past the staleness threshold directly,
	// rather than waiting presenceStaleAfter (60s) in a test.
	hub.mu.Lock()
	info := hub.presence[boardID][staleUser]
	info.LastHeartbeat = time.Now().Add(-2 * presenceStaleAfter)
	hub.presence[boardID][staleUser] = info
	hub.mu.Unlock()

	ch := hub.subscribe(boardID)
	defer hub.unsubscribe(boardID, ch)

	hub.reapStale()

	list := hub.GetPresence(boardID)
	require.Len(t, list, 1)
	require.Equal(t, freshUser, list[0].UserID)

	select {
	case event := <-ch:
		require.Equal(t, "presence.left", event.Type)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for presence.left event from reaper")
	}
}

func TestHub_StartReaper_StopsOnContextCancel(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		hub.StartReaper(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StartReaper did not return after context cancellation")
	}
}

func TestHub_Close_ClearsPresence(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	userID := uuid.New()

	hub.JoinPresence(boardID, userID, "Ada")
	require.Len(t, hub.GetPresence(boardID), 1)

	hub.Close()

	require.Empty(t, hub.GetPresence(boardID))
}
