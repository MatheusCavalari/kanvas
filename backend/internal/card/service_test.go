package card

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestService_CreateColumn_RequiresMembership(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
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
	svc := NewService(repo, boardAuth)
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
	svc := NewService(repo, boardAuth)
	ctx := context.Background()

	err := svc.DeleteColumn(ctx, uuid.New(), uuid.New())
	require.True(t, errors.Is(err, ErrColumnNotFound))
}

func TestService_ReorderColumns_PersistsOrder(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
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
	svc := NewService(repo, boardAuth)
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
