package board

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestService_CreateBoard_AddsOwnerAsMember(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Sprint Board")

	require.NoError(t, err)
	require.Equal(t, "Sprint Board", board.Name)
	require.Equal(t, owner, board.OwnerID)

	member, err := repo.GetMember(ctx, board.ID, owner)
	require.NoError(t, err)
	require.Equal(t, RoleOwner, member.Role)
}

func TestService_ListBoards_OnlyReturnsMemberBoards(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	other := uuid.New()

	_, err := svc.CreateBoard(ctx, owner, "Owner's Board")
	require.NoError(t, err)

	boards, err := svc.ListBoards(ctx, other)
	require.NoError(t, err)
	require.Empty(t, boards)

	boards, err = svc.ListBoards(ctx, owner)
	require.NoError(t, err)
	require.Len(t, boards, 1)
}

func TestService_GetBoard_NonMemberForbidden(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	stranger := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Sprint Board")
	require.NoError(t, err)

	_, err = svc.GetBoard(ctx, board.ID, stranger)
	require.True(t, errors.Is(err, ErrNotAMember))

	fetched, err := svc.GetBoard(ctx, board.ID, owner)
	require.NoError(t, err)
	require.Equal(t, board.ID, fetched.ID)
}
