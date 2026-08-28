package activity

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type fakeRepo struct {
	mu        sync.Mutex
	entries   []Entry
	insertErr error
}

func (f *fakeRepo) Insert(_ context.Context, e Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return f.insertErr
	}
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeRepo) ListByBoard(_ context.Context, boardID uuid.UUID, cursor time.Time, limit int) ([]Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Entry
	for _, e := range f.entries {
		if e.BoardID == boardID && e.CreatedAt.Before(cursor) {
			out = append(out, e)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) all() []Entry {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Entry, len(f.entries))
	copy(out, f.entries)
	return out
}

type fakeNext struct {
	mu    sync.Mutex
	calls []fakePublishCall
}

type fakePublishCall struct {
	boardID   uuid.UUID
	eventType string
	payload   interface{}
}

func (f *fakeNext) Publish(_ context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakePublishCall{boardID: boardID, eventType: eventType, payload: payload})
}

func (f *fakeNext) all() []fakePublishCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakePublishCall, len(f.calls))
	copy(out, f.calls)
	return out
}

type structPayload struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

func TestRecorder_RecordsEntryWithStructPayload(t *testing.T) {
	repo := &fakeRepo{}
	next := &fakeNext{}
	rec := NewRecorder(repo, next)

	actorID := uuid.New()
	boardID := uuid.New()
	entityID := uuid.New()
	ctx := middleware.ContextWithUserID(context.Background(), actorID)

	payload := structPayload{ID: entityID, Name: "a label"}
	rec.Publish(ctx, boardID, "label.created", payload)

	entries := repo.all()
	require.Len(t, entries, 1)
	e := entries[0]
	require.Equal(t, boardID, e.BoardID)
	require.Equal(t, actorID, e.ActorID)
	require.Equal(t, "label", e.EntityType)
	require.Equal(t, "created", e.Action)
	require.Equal(t, entityID, e.EntityID)

	var decoded structPayload
	require.NoError(t, json.Unmarshal(e.SnapshotAfter, &decoded))
	require.Equal(t, payload, decoded)
}

func TestRecorder_RecordsEntryWithMapPayload(t *testing.T) {
	repo := &fakeRepo{}
	next := &fakeNext{}
	rec := NewRecorder(repo, next)

	actorID := uuid.New()
	boardID := uuid.New()
	entityID := uuid.New()
	ctx := middleware.ContextWithUserID(context.Background(), actorID)

	payload := map[string]interface{}{"id": entityID, "board_id": boardID}
	rec.Publish(ctx, boardID, "label.deleted", payload)

	entries := repo.all()
	require.Len(t, entries, 1)
	require.Equal(t, "label", entries[0].EntityType)
	require.Equal(t, "deleted", entries[0].Action)
	require.Equal(t, entityID, entries[0].EntityID)
}

func TestRecorder_FallsBackToBoardIDWhenNoEntityID(t *testing.T) {
	repo := &fakeRepo{}
	next := &fakeNext{}
	rec := NewRecorder(repo, next)

	actorID := uuid.New()
	boardID := uuid.New()
	ctx := middleware.ContextWithUserID(context.Background(), actorID)

	type bulkPayload struct {
		BoardID   uuid.UUID   `json:"board_id"`
		ColumnIDs []uuid.UUID `json:"column_ids"`
	}
	rec.Publish(ctx, boardID, "column.reordered", bulkPayload{BoardID: boardID})

	entries := repo.all()
	require.Len(t, entries, 1)
	require.Equal(t, boardID, entries[0].EntityID)
}

func TestRecorder_SkipsRecordingWithoutActorButStillDelegates(t *testing.T) {
	repo := &fakeRepo{}
	next := &fakeNext{}
	rec := NewRecorder(repo, next)

	boardID := uuid.New()
	payload := structPayload{ID: uuid.New(), Name: "x"}

	rec.Publish(context.Background(), boardID, "label.created", payload)

	require.Empty(t, repo.all())

	calls := next.all()
	require.Len(t, calls, 1)
	require.Equal(t, "label.created", calls[0].eventType)
}

func TestRecorder_DelegatesOriginalEventAndPublishesActivityCreated(t *testing.T) {
	repo := &fakeRepo{}
	next := &fakeNext{}
	rec := NewRecorder(repo, next)

	actorID := uuid.New()
	boardID := uuid.New()
	ctx := middleware.ContextWithUserID(context.Background(), actorID)

	payload := structPayload{ID: uuid.New(), Name: "x"}
	rec.Publish(ctx, boardID, "label.created", payload)

	calls := next.all()
	require.Len(t, calls, 2)
	require.Equal(t, "label.created", calls[0].eventType)
	require.Equal(t, payload, calls[0].payload)
	require.Equal(t, EventActivityCreated, calls[1].eventType)
}

func TestRecorder_InsertFailureIsNonFatal(t *testing.T) {
	repo := &fakeRepo{insertErr: assertErr}
	next := &fakeNext{}
	rec := NewRecorder(repo, next)

	actorID := uuid.New()
	boardID := uuid.New()
	ctx := middleware.ContextWithUserID(context.Background(), actorID)

	require.NotPanics(t, func() {
		rec.Publish(ctx, boardID, "label.created", structPayload{ID: uuid.New()})
	})

	calls := next.all()
	require.Len(t, calls, 1, "original event should still be delegated even if recording fails")
}

var assertErr = errStub("insert failed")

type errStub string

func (e errStub) Error() string { return string(e) }
