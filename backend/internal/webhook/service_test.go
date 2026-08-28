package webhook_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/webhook"
)

func newTestService() (*webhook.Service, *fakeRepo, *fakeOwnerChecker) {
	repo := newFakeRepo()
	owner := newFakeOwnerChecker()
	svc := webhook.NewService(repo, owner)
	return svc, repo, owner
}

func TestService_Create_ByOwner(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	ownerID := uuid.New()
	owner.owners[ownerID] = true

	w, err := svc.Create(ctx, boardID, ownerID, "https://example.com/hook", []string{"card.created"})
	require.NoError(t, err)
	require.Equal(t, boardID, w.BoardID)
	require.Equal(t, ownerID, w.OwnerID)
	require.True(t, w.Active)
	require.Len(t, w.Secret, 64)
	require.Equal(t, []string{"card.created"}, w.Events)
}

func TestService_Create_ByNonOwner_Fails(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	member := uuid.New()
	owner.owners[member] = false

	_, err := svc.Create(ctx, boardID, member, "https://example.com/hook", []string{"card.created"})
	require.ErrorIs(t, err, webhook.ErrForbidden)
}

func TestService_Create_InvalidURL(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	ownerID := uuid.New()
	owner.owners[ownerID] = true

	_, err := svc.Create(ctx, boardID, ownerID, "not-a-url", []string{"card.created"})
	require.ErrorIs(t, err, webhook.ErrInvalidURL)

	_, err = svc.Create(ctx, boardID, ownerID, "ftp://example.com/hook", []string{"card.created"})
	require.ErrorIs(t, err, webhook.ErrInvalidURL)
}

func TestService_Create_InvalidEvents(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	ownerID := uuid.New()
	owner.owners[ownerID] = true

	_, err := svc.Create(ctx, boardID, ownerID, "https://example.com/hook", nil)
	require.ErrorIs(t, err, webhook.ErrInvalidEvents)

	_, err = svc.Create(ctx, boardID, ownerID, "https://example.com/hook", []string{"  "})
	require.ErrorIs(t, err, webhook.ErrInvalidEvents)
}

func TestService_Update_ByOwner(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	ownerID := uuid.New()
	owner.owners[ownerID] = true

	w, err := svc.Create(ctx, boardID, ownerID, "https://example.com/hook", []string{"card.created"})
	require.NoError(t, err)

	updated, err := svc.Update(ctx, boardID, w.ID, ownerID, "https://example.com/hook2", []string{"card.updated"}, false)
	require.NoError(t, err)
	require.Equal(t, "https://example.com/hook2", updated.URL)
	require.Equal(t, []string{"card.updated"}, updated.Events)
	require.False(t, updated.Active)
}

func TestService_Update_ByNonOwner_Fails(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	ownerID := uuid.New()
	member := uuid.New()
	owner.owners[ownerID] = true

	w, err := svc.Create(ctx, boardID, ownerID, "https://example.com/hook", []string{"card.created"})
	require.NoError(t, err)

	_, err = svc.Update(ctx, boardID, w.ID, member, "https://example.com/hook2", []string{"card.updated"}, true)
	require.ErrorIs(t, err, webhook.ErrForbidden)
}

func TestService_Update_WrongBoard_NotFound(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	otherBoardID := uuid.New()
	ownerID := uuid.New()
	owner.owners[ownerID] = true

	w, err := svc.Create(ctx, boardID, ownerID, "https://example.com/hook", []string{"card.created"})
	require.NoError(t, err)

	_, err = svc.Update(ctx, otherBoardID, w.ID, ownerID, "https://example.com/hook2", []string{"card.updated"}, true)
	require.ErrorIs(t, err, webhook.ErrWebhookNotFound)
}

func TestService_Delete_ByOwner(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	ownerID := uuid.New()
	owner.owners[ownerID] = true

	w, err := svc.Create(ctx, boardID, ownerID, "https://example.com/hook", []string{"card.created"})
	require.NoError(t, err)

	require.NoError(t, svc.Delete(ctx, boardID, w.ID, ownerID))

	list, err := svc.ListByBoard(ctx, boardID, ownerID)
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestService_Delete_ByNonOwner_Fails(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	ownerID := uuid.New()
	member := uuid.New()
	owner.owners[ownerID] = true

	w, err := svc.Create(ctx, boardID, ownerID, "https://example.com/hook", []string{"card.created"})
	require.NoError(t, err)

	err = svc.Delete(ctx, boardID, w.ID, member)
	require.ErrorIs(t, err, webhook.ErrForbidden)
}

func TestService_ListByBoard_ByNonOwner_Fails(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	member := uuid.New()
	owner.owners[member] = false

	_, err := svc.ListByBoard(ctx, boardID, member)
	require.ErrorIs(t, err, webhook.ErrForbidden)
}

func TestService_ListDeliveries_ByOwner(t *testing.T) {
	svc, repo, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	ownerID := uuid.New()
	owner.owners[ownerID] = true

	w, err := svc.Create(ctx, boardID, ownerID, "https://example.com/hook", []string{"card.created"})
	require.NoError(t, err)

	_, err = repo.CreateDelivery(ctx, webhook.Delivery{ID: uuid.New(), WebhookID: w.ID, EventType: "card.created", Payload: []byte(`{}`), Status: webhook.DeliveryStatusSuccess})
	require.NoError(t, err)

	deliveries, err := svc.ListDeliveries(ctx, boardID, w.ID, ownerID, farFuture(), 20)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
}

func TestService_ListDeliveries_WrongBoard_NotFound(t *testing.T) {
	svc, _, owner := newTestService()
	ctx := context.Background()

	boardID := uuid.New()
	otherBoardID := uuid.New()
	ownerID := uuid.New()
	owner.owners[ownerID] = true

	w, err := svc.Create(ctx, boardID, ownerID, "https://example.com/hook", []string{"card.created"})
	require.NoError(t, err)

	_, err = svc.ListDeliveries(ctx, otherBoardID, w.ID, ownerID, farFuture(), 20)
	require.ErrorIs(t, err, webhook.ErrWebhookNotFound)
}
