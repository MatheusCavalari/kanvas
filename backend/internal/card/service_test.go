package card

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestService_CreateColumn_RequiresMembership(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	stranger := uuid.New()
	boardAuth.addMember(boardID, member)

	_, err := svc.CreateColumn(ctx, boardID, stranger, "To Do")
	require.Error(t, err)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	require.Equal(t, "To Do", column.Title)
	require.Equal(t, boardID, column.BoardID)
}

func TestService_RenameColumn_RequiresMembership(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)

	renamed, err := svc.RenameColumn(ctx, column.ID, member, "Backlog")
	require.NoError(t, err)
	require.Equal(t, "Backlog", renamed.Title)

	_, err = svc.RenameColumn(ctx, column.ID, uuid.New(), "Hacked")
	require.Error(t, err)
}

func TestService_DeleteColumn_UnknownColumn(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()

	err := svc.DeleteColumn(ctx, uuid.New(), uuid.New())
	require.True(t, errors.Is(err, ErrColumnNotFound))
}

func TestService_ReorderColumns_PersistsOrder(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	first, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	second, err := svc.CreateColumn(ctx, boardID, member, "Doing")
	require.NoError(t, err)

	err = svc.ReorderColumns(ctx, boardID, member, []uuid.UUID{second.ID, first.ID})
	require.NoError(t, err)

	columns, err := svc.ListBoardColumns(ctx, boardID, member)
	require.NoError(t, err)
	require.Len(t, columns, 2)
	require.Equal(t, second.ID, columns[0].ID)
	require.Equal(t, first.ID, columns[1].ID)
}

func TestService_ListBoardColumns_IncludesCards(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	_, err = repo.CreateCard(ctx, Card{ID: uuid.New(), ColumnID: column.ID, Title: "Write plan"})
	require.NoError(t, err)

	columns, err := svc.ListBoardColumns(ctx, boardID, member)
	require.NoError(t, err)
	require.Len(t, columns, 1)
	require.Len(t, columns[0].Cards, 1)
	require.Equal(t, "Write plan", columns[0].Cards[0].Title)
}

func TestService_CreateCard_Success(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)

	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "details", nil, nil)
	require.NoError(t, err)
	require.Equal(t, "Write plan", card.Title)
	require.Equal(t, column.ID, card.ColumnID)
}

func TestService_CreateCard_UnknownColumn(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()

	_, err := svc.CreateCard(ctx, uuid.New(), uuid.New(), "Write plan", "", nil, nil)
	require.True(t, errors.Is(err, ErrColumnNotFound))
}

func TestService_UpdateCard_RequiresMembership(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)

	due := time.Now().Add(24 * time.Hour)
	updated, err := svc.UpdateCard(ctx, card.ID, member, "Write plan v2", "updated details", nil, &due)
	require.NoError(t, err)
	require.Equal(t, "Write plan v2", updated.Title)
	require.Equal(t, "updated details", updated.Description)

	_, err = svc.UpdateCard(ctx, card.ID, uuid.New(), "Hacked", "", nil, nil)
	require.Error(t, err)
}

func TestService_DeleteCard_Success(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)

	err = svc.DeleteCard(ctx, card.ID, member)
	require.NoError(t, err)

	_, err = repo.GetCardByID(ctx, card.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_MoveCard_WithinSameColumn(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	first, err := svc.CreateCard(ctx, column.ID, member, "First", "", nil, nil)
	require.NoError(t, err)
	second, err := svc.CreateCard(ctx, column.ID, member, "Second", "", nil, nil)
	require.NoError(t, err)

	_, err = svc.MoveCard(ctx, first.ID, member, column.ID, 1)
	require.NoError(t, err)

	cards, err := repo.ListCardsByColumn(ctx, column.ID)
	require.NoError(t, err)
	require.Len(t, cards, 2)
	require.Equal(t, second.ID, cards[0].ID)
	require.Equal(t, first.ID, cards[1].ID)
}

func TestService_MoveCard_AcrossColumns(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	todo, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	doing, err := svc.CreateColumn(ctx, boardID, member, "Doing")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, todo.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)

	moved, err := svc.MoveCard(ctx, card.ID, member, doing.ID, 0)
	require.NoError(t, err)
	require.Equal(t, doing.ID, moved.ColumnID)

	todoCards, err := repo.ListCardsByColumn(ctx, todo.ID)
	require.NoError(t, err)
	require.Empty(t, todoCards)

	doingCards, err := repo.ListCardsByColumn(ctx, doing.ID)
	require.NoError(t, err)
	require.Len(t, doingCards, 1)
	require.Equal(t, card.ID, doingCards[0].ID)
}

func TestService_MoveCard_AcrossColumns_RenumbersRemainingSourceCards(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	todo, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	doing, err := svc.CreateColumn(ctx, boardID, member, "Doing")
	require.NoError(t, err)

	first, err := svc.CreateCard(ctx, todo.ID, member, "First", "", nil, nil)
	require.NoError(t, err)
	second, err := svc.CreateCard(ctx, todo.ID, member, "Second", "", nil, nil)
	require.NoError(t, err)
	third, err := svc.CreateCard(ctx, todo.ID, member, "Third", "", nil, nil)
	require.NoError(t, err)

	// Move the MIDDLE card out, leaving a gap at its old position.
	_, err = svc.MoveCard(ctx, second.ID, member, doing.ID, 0)
	require.NoError(t, err)

	remaining, err := repo.ListCardsByColumn(ctx, todo.ID)
	require.NoError(t, err)
	require.Len(t, remaining, 2)
	require.Equal(t, first.ID, remaining[0].ID)
	require.Equal(t, int32(0), remaining[0].Position)
	require.Equal(t, third.ID, remaining[1].ID)
	require.Equal(t, int32(1), remaining[1].Position)
}

func TestService_MoveCard_RejectsCrossBoardMove(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardA := uuid.New()
	boardB := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardA, member)
	boardAuth.addMember(boardB, member)

	columnA, err := svc.CreateColumn(ctx, boardA, member, "To Do")
	require.NoError(t, err)
	columnB, err := svc.CreateColumn(ctx, boardB, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, columnA.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)

	_, err = svc.MoveCard(ctx, card.ID, member, columnB.ID, 0)
	require.True(t, errors.Is(err, ErrColumnNotFound))
}

func TestService_DeleteColumn_RenumbersRemainingSiblings(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	first, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	second, err := svc.CreateColumn(ctx, boardID, member, "Doing")
	require.NoError(t, err)
	third, err := svc.CreateColumn(ctx, boardID, member, "Done")
	require.NoError(t, err)

	// Delete the MIDDLE column, leaving a gap at its old position.
	err = svc.DeleteColumn(ctx, second.ID, member)
	require.NoError(t, err)

	remaining, err := repo.ListColumnsByBoard(ctx, boardID)
	require.NoError(t, err)
	require.Len(t, remaining, 2)
	require.Equal(t, first.ID, remaining[0].ID)
	require.Equal(t, int32(0), remaining[0].Position)
	require.Equal(t, third.ID, remaining[1].ID)
	require.Equal(t, int32(1), remaining[1].Position)
}

func TestService_DeleteCard_RenumbersRemainingSiblings(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	first, err := svc.CreateCard(ctx, column.ID, member, "First", "", nil, nil)
	require.NoError(t, err)
	second, err := svc.CreateCard(ctx, column.ID, member, "Second", "", nil, nil)
	require.NoError(t, err)
	third, err := svc.CreateCard(ctx, column.ID, member, "Third", "", nil, nil)
	require.NoError(t, err)

	// Delete the MIDDLE card, leaving a gap at its old position.
	err = svc.DeleteCard(ctx, second.ID, member)
	require.NoError(t, err)

	remaining, err := repo.ListCardsByColumn(ctx, column.ID)
	require.NoError(t, err)
	require.Len(t, remaining, 2)
	require.Equal(t, first.ID, remaining[0].ID)
	require.Equal(t, int32(0), remaining[0].Position)
	require.Equal(t, third.ID, remaining[1].ID)
	require.Equal(t, int32(1), remaining[1].Position)
}

func TestService_ReorderColumns_RejectsPartialList(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	first, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	_, err = svc.CreateColumn(ctx, boardID, member, "Doing")
	require.NoError(t, err)

	err = svc.ReorderColumns(ctx, boardID, member, []uuid.UUID{first.ID})
	require.ErrorIs(t, err, ErrInvalidReorder)
}

func TestService_ReorderColumns_RejectsDuplicateID(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	first, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	second, err := svc.CreateColumn(ctx, boardID, member, "Doing")
	require.NoError(t, err)

	err = svc.ReorderColumns(ctx, boardID, member, []uuid.UUID{first.ID, first.ID})
	require.ErrorIs(t, err, ErrInvalidReorder)
	_ = second
}

func TestService_ReorderColumns_RejectsUnknownID(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth, newFakeEventPublisher())
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	first, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	second, err := svc.CreateColumn(ctx, boardID, member, "Doing")
	require.NoError(t, err)

	err = svc.ReorderColumns(ctx, boardID, member, []uuid.UUID{first.ID, second.ID, uuid.New()})
	require.ErrorIs(t, err, ErrInvalidReorder)
}

func TestReorderWithInsert(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	cards := []Card{{ID: a}, {ID: b}, {ID: c}}

	result := reorderWithInsert(cards, b, 0)

	require.Equal(t, []uuid.UUID{b, a, c}, result)
}

func TestService_CreateColumn_PublishesEvent(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	events := newFakeEventPublisher()
	svc := NewService(repo, boardAuth, events)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	_, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)

	require.Len(t, events.events, 1)
	require.Equal(t, EventColumnCreated, events.events[0].eventType)
	require.Equal(t, boardID, events.events[0].boardID)
}

func TestService_DeleteCard_PublishesEvent(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	events := newFakeEventPublisher()
	svc := NewService(repo, boardAuth, events)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)
	events.reset()

	err = svc.DeleteCard(ctx, card.ID, member)
	require.NoError(t, err)

	require.Len(t, events.events, 1)
	require.Equal(t, EventCardDeleted, events.events[0].eventType)
	require.Equal(t, boardID, events.events[0].boardID)
}

func TestService_MoveCard_PublishesEvent(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	events := newFakeEventPublisher()
	svc := NewService(repo, boardAuth, events)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)
	events.reset()

	_, err = svc.MoveCard(ctx, card.ID, member, column.ID, 0)
	require.NoError(t, err)

	require.Len(t, events.events, 1)
	require.Equal(t, EventCardMoved, events.events[0].eventType)
	require.Equal(t, boardID, events.events[0].boardID)
}
