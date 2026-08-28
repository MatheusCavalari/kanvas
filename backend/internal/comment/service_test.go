package comment_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/comment"
)

func newTestService() (*comment.Service, *fakeRepo, *fakeCardLookup, *fakeBoardAuth, *fakeOwnerChecker, *fakeEventPublisher) {
	repo := newFakeRepo()
	cards := newFakeCardLookup()
	auth := &fakeBoardAuth{allowed: map[uuid.UUID]bool{}}
	owner := newFakeOwnerChecker()
	events := &fakeEventPublisher{}
	svc := comment.NewService(repo, cards, auth, owner, events)
	return svc, repo, cards, auth, owner, events
}

func TestService_CreateAndListByCard(t *testing.T) {
	svc, _, cards, auth, _, events := newTestService()
	ctx := context.Background()

	userID := uuid.New()
	boardID := uuid.New()
	cardID := uuid.New()
	cards.boards[cardID] = boardID
	auth.allowed[userID] = true

	c, err := svc.Create(ctx, cardID, userID, "First comment")
	require.NoError(t, err)
	require.Equal(t, cardID, c.CardID)
	require.Equal(t, userID, c.AuthorID)
	require.Equal(t, "First comment", c.Body)
	require.True(t, events.has(comment.EventCommentCreated))

	list, err := svc.ListByCard(ctx, cardID, userID, time.Time{}, 20)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, c.ID, list[0].ID)
}

func TestService_Create_NotMember(t *testing.T) {
	svc, _, cards, _, _, _ := newTestService()
	ctx := context.Background()

	userID := uuid.New()
	boardID := uuid.New()
	cardID := uuid.New()
	cards.boards[cardID] = boardID

	_, err := svc.Create(ctx, cardID, userID, "body")
	require.Error(t, err)
}

func TestService_Create_InvalidBody(t *testing.T) {
	svc, _, cards, auth, _, _ := newTestService()
	ctx := context.Background()

	userID := uuid.New()
	boardID := uuid.New()
	cardID := uuid.New()
	cards.boards[cardID] = boardID
	auth.allowed[userID] = true

	_, err := svc.Create(ctx, cardID, userID, "")
	require.ErrorIs(t, err, comment.ErrInvalidBody)
}

func TestService_Update_ByAuthor(t *testing.T) {
	svc, _, cards, auth, _, events := newTestService()
	ctx := context.Background()

	author := uuid.New()
	boardID := uuid.New()
	cardID := uuid.New()
	cards.boards[cardID] = boardID
	auth.allowed[author] = true

	c, err := svc.Create(ctx, cardID, author, "original")
	require.NoError(t, err)

	updated, err := svc.Update(ctx, c.ID, author, "edited")
	require.NoError(t, err)
	require.Equal(t, "edited", updated.Body)
	require.True(t, events.has(comment.EventCommentUpdated))
}

func TestService_Update_ByNonAuthor_Fails(t *testing.T) {
	svc, _, cards, auth, _, _ := newTestService()
	ctx := context.Background()

	author := uuid.New()
	otherMember := uuid.New()
	boardID := uuid.New()
	cardID := uuid.New()
	cards.boards[cardID] = boardID
	auth.allowed[author] = true
	auth.allowed[otherMember] = true

	c, err := svc.Create(ctx, cardID, author, "original")
	require.NoError(t, err)

	_, err = svc.Update(ctx, c.ID, otherMember, "hijacked")
	require.ErrorIs(t, err, comment.ErrNotAuthor)
}

func TestService_Delete_ByAuthor(t *testing.T) {
	svc, _, cards, auth, _, events := newTestService()
	ctx := context.Background()

	author := uuid.New()
	boardID := uuid.New()
	cardID := uuid.New()
	cards.boards[cardID] = boardID
	auth.allowed[author] = true

	c, err := svc.Create(ctx, cardID, author, "to delete")
	require.NoError(t, err)

	err = svc.Delete(ctx, c.ID, author)
	require.NoError(t, err)
	require.True(t, events.has(comment.EventCommentDeleted))

	_, err = svc.Update(ctx, c.ID, author, "x")
	require.ErrorIs(t, err, comment.ErrCommentNotFound)
}

func TestService_Delete_ByBoardOwner(t *testing.T) {
	svc, _, cards, auth, owner, events := newTestService()
	ctx := context.Background()

	author := uuid.New()
	boardOwner := uuid.New()
	boardID := uuid.New()
	cardID := uuid.New()
	cards.boards[cardID] = boardID
	auth.allowed[author] = true
	auth.allowed[boardOwner] = true
	owner.owners[boardOwner] = true

	c, err := svc.Create(ctx, cardID, author, "to delete")
	require.NoError(t, err)

	err = svc.Delete(ctx, c.ID, boardOwner)
	require.NoError(t, err)
	require.True(t, events.has(comment.EventCommentDeleted))
}

func TestService_Delete_ByStranger_Fails(t *testing.T) {
	svc, _, cards, auth, _, _ := newTestService()
	ctx := context.Background()

	author := uuid.New()
	otherMember := uuid.New()
	boardID := uuid.New()
	cardID := uuid.New()
	cards.boards[cardID] = boardID
	auth.allowed[author] = true
	auth.allowed[otherMember] = true

	c, err := svc.Create(ctx, cardID, author, "keep")
	require.NoError(t, err)

	err = svc.Delete(ctx, c.ID, otherMember)
	require.ErrorIs(t, err, comment.ErrForbidden)
}

func TestService_ListByCard_CursorPagination(t *testing.T) {
	svc, repo, cards, auth, _, _ := newTestService()
	ctx := context.Background()

	userID := uuid.New()
	boardID := uuid.New()
	cardID := uuid.New()
	cards.boards[cardID] = boardID
	auth.allowed[userID] = true

	base := time.Now().Add(-time.Hour)
	var created []time.Time
	for i := 0; i < 5; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		created = append(created, ts)
		_, err := repo.Create(ctx, mustComment(cardID, userID, ts))
		require.NoError(t, err)
	}

	// First page: limit 2, from the beginning.
	page1, err := svc.ListByCard(ctx, cardID, userID, time.Time{}, 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.True(t, page1[0].CreatedAt.Equal(created[0]))
	require.True(t, page1[1].CreatedAt.Equal(created[1]))

	// Second page: cursor is the last item's created_at from page1.
	page2, err := svc.ListByCard(ctx, cardID, userID, page1[len(page1)-1].CreatedAt, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	require.True(t, page2[0].CreatedAt.Equal(created[2]))
	require.True(t, page2[1].CreatedAt.Equal(created[3]))

	// Third page: remaining single item.
	page3, err := svc.ListByCard(ctx, cardID, userID, page2[len(page2)-1].CreatedAt, 2)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	require.True(t, page3[0].CreatedAt.Equal(created[4]))
}

func mustComment(cardID, authorID uuid.UUID, createdAt time.Time) comment.Comment {
	return comment.Comment{
		ID:        uuid.New(),
		CardID:    cardID,
		AuthorID:  authorID,
		Body:      "body",
		CreatedAt: createdAt,
	}
}
