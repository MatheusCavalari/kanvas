package label_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/label"
)

func TestService_CreateLabel(t *testing.T) {
	repo := newFakeRepo()
	events := &fakeEventPublisher{}
	userID := uuid.New()
	boardID := uuid.New()
	auth := &fakeBoardAuth{allowed: map[uuid.UUID]bool{userID: true}}
	svc := label.NewService(repo, auth, events)

	l, err := svc.Create(context.Background(), boardID, userID, "Bug", "#FF0000")
	require.NoError(t, err)
	require.Equal(t, "Bug", l.Name)
	require.Equal(t, "#FF0000", l.Color)
	require.Equal(t, boardID, l.BoardID)
	require.Contains(t, events.events, "label.created")
}

func TestService_CreateLabel_InvalidColor(t *testing.T) {
	repo := newFakeRepo()
	events := &fakeEventPublisher{}
	userID := uuid.New()
	auth := &fakeBoardAuth{allowed: map[uuid.UUID]bool{userID: true}}
	svc := label.NewService(repo, auth, events)

	_, err := svc.Create(context.Background(), uuid.New(), userID, "Bug", "red")
	require.ErrorIs(t, err, label.ErrInvalidColor)
}

func TestService_AttachDetach(t *testing.T) {
	repo := newFakeRepo()
	events := &fakeEventPublisher{}
	userID := uuid.New()
	boardID := uuid.New()
	auth := &fakeBoardAuth{allowed: map[uuid.UUID]bool{userID: true}}
	svc := label.NewService(repo, auth, events)

	l, err := svc.Create(context.Background(), boardID, userID, "Feature", "#00FF00")
	require.NoError(t, err)

	cardID := uuid.New()
	err = svc.AttachToCard(context.Background(), cardID, l.ID, userID)
	require.NoError(t, err)
	require.Contains(t, events.events, "card.label_added")

	labels, err := svc.ListCardLabels(context.Background(), cardID)
	require.NoError(t, err)
	require.Len(t, labels, 1)

	err = svc.DetachFromCard(context.Background(), cardID, l.ID, userID)
	require.NoError(t, err)
	require.Contains(t, events.events, "card.label_removed")
}

func TestService_Update(t *testing.T) {
	repo := newFakeRepo()
	events := &fakeEventPublisher{}
	userID := uuid.New()
	boardID := uuid.New()
	auth := &fakeBoardAuth{allowed: map[uuid.UUID]bool{userID: true}}
	svc := label.NewService(repo, auth, events)

	l, err := svc.Create(context.Background(), boardID, userID, "Bug", "#FF0000")
	require.NoError(t, err)

	updated, err := svc.Update(context.Background(), l.ID, userID, "Critical Bug", "#AA0000")
	require.NoError(t, err)
	require.Equal(t, "Critical Bug", updated.Name)
	require.Equal(t, "#AA0000", updated.Color)
	require.Contains(t, events.events, "label.updated")
}

func TestService_Update_NotFound(t *testing.T) {
	repo := newFakeRepo()
	events := &fakeEventPublisher{}
	userID := uuid.New()
	auth := &fakeBoardAuth{allowed: map[uuid.UUID]bool{userID: true}}
	svc := label.NewService(repo, auth, events)

	_, err := svc.Update(context.Background(), uuid.New(), userID, "x", "#FFFFFF")
	require.ErrorIs(t, err, label.ErrLabelNotFound)
}

func TestService_Delete(t *testing.T) {
	repo := newFakeRepo()
	events := &fakeEventPublisher{}
	userID := uuid.New()
	boardID := uuid.New()
	auth := &fakeBoardAuth{allowed: map[uuid.UUID]bool{userID: true}}
	svc := label.NewService(repo, auth, events)

	l, err := svc.Create(context.Background(), boardID, userID, "Bug", "#FF0000")
	require.NoError(t, err)

	err = svc.Delete(context.Background(), l.ID, userID)
	require.NoError(t, err)
	require.Contains(t, events.events, "label.deleted")

	_, err = svc.Update(context.Background(), l.ID, userID, "x", "#FFFFFF")
	require.ErrorIs(t, err, label.ErrLabelNotFound)
}

func TestService_ListByBoard_Forbidden(t *testing.T) {
	repo := newFakeRepo()
	events := &fakeEventPublisher{}
	userID := uuid.New()
	stranger := uuid.New()
	boardID := uuid.New()
	auth := &fakeBoardAuth{allowed: map[uuid.UUID]bool{userID: true}}
	svc := label.NewService(repo, auth, events)

	_, err := svc.Create(context.Background(), boardID, userID, "Bug", "#FF0000")
	require.NoError(t, err)

	_, err = svc.ListByBoard(context.Background(), boardID, stranger)
	require.Error(t, err)
}
