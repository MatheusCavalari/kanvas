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

func TestService_RenameBoard_RequiresMembership(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Old Name")
	require.NoError(t, err)

	renamed, err := svc.RenameBoard(ctx, board.ID, owner, "New Name")
	require.NoError(t, err)
	require.Equal(t, "New Name", renamed.Name)

	_, err = svc.RenameBoard(ctx, board.ID, uuid.New(), "Hacked Name")
	require.True(t, errors.Is(err, ErrNotAMember))
}

func TestService_DeleteBoard_OnlyOwner(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "To Delete")
	require.NoError(t, err)

	err = svc.DeleteBoard(ctx, board.ID, uuid.New())
	require.True(t, errors.Is(err, ErrNotAMember))

	err = svc.DeleteBoard(ctx, board.ID, owner)
	require.NoError(t, err)

	_, err = repo.GetBoardByID(ctx, board.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_DeleteBoard_MemberForbidden(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	member := uuid.New()
	users.usersByEmail["member@example.com"] = member

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)
	_, err = svc.InviteMember(ctx, board.ID, owner, "member@example.com")
	require.NoError(t, err)

	// member is an actual member of the board (not a stranger), so this
	// must reach and fail the role check (ErrForbidden), not the
	// membership check (ErrNotAMember).
	err = svc.DeleteBoard(ctx, board.ID, member)
	require.True(t, errors.Is(err, ErrForbidden))
	require.False(t, errors.Is(err, ErrNotAMember))

	_, err = repo.GetBoardByID(ctx, board.ID)
	require.NoError(t, err)
}

func TestService_RemoveMember_MemberForbidden(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	member := uuid.New()
	other := uuid.New()
	users.usersByEmail["member@example.com"] = member
	users.usersByEmail["other@example.com"] = other

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)
	_, err = svc.InviteMember(ctx, board.ID, owner, "member@example.com")
	require.NoError(t, err)
	_, err = svc.InviteMember(ctx, board.ID, owner, "other@example.com")
	require.NoError(t, err)

	// member is an actual member of the board (not a stranger), so this
	// must reach and fail the role check (ErrForbidden), not the
	// membership check (ErrNotAMember).
	err = svc.RemoveMember(ctx, board.ID, member, other)
	require.True(t, errors.Is(err, ErrForbidden))
	require.False(t, errors.Is(err, ErrNotAMember))

	_, err = repo.GetMember(ctx, board.ID, other)
	require.NoError(t, err)
}

func TestService_InviteMember_Success(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	invitee := uuid.New()
	users.usersByEmail["friend@example.com"] = invitee

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)

	member, err := svc.InviteMember(ctx, board.ID, owner, "Friend@Example.com")
	require.NoError(t, err)
	require.Equal(t, invitee, member.UserID)
	require.Equal(t, RoleMember, member.Role)
}

func TestService_InviteMember_NotOwnerForbidden(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	member := uuid.New()
	users.usersByEmail["member@example.com"] = member
	users.usersByEmail["another@example.com"] = uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)
	_, err = svc.InviteMember(ctx, board.ID, owner, "member@example.com")
	require.NoError(t, err)

	_, err = svc.InviteMember(ctx, board.ID, member, "another@example.com")
	require.True(t, errors.Is(err, ErrForbidden))
}

func TestService_InviteMember_AlreadyMember(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	users.usersByEmail["owner@example.com"] = owner

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)

	_, err = svc.InviteMember(ctx, board.ID, owner, "owner@example.com")
	require.True(t, errors.Is(err, ErrAlreadyMember))
}

func TestService_InviteMember_UnknownEmail(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)

	_, err = svc.InviteMember(ctx, board.ID, owner, "nobody@example.com")
	require.True(t, errors.Is(err, ErrMemberUserNotFound))
}

func TestService_RemoveMember_CannotRemoveSelf(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)

	err = svc.RemoveMember(ctx, board.ID, owner, owner)
	require.True(t, errors.Is(err, ErrCannotRemoveOwner))
}

func TestService_RemoveMember_Success(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	invitee := uuid.New()
	users.usersByEmail["friend@example.com"] = invitee

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)
	_, err = svc.InviteMember(ctx, board.ID, owner, "friend@example.com")
	require.NoError(t, err)

	err = svc.RemoveMember(ctx, board.ID, owner, invitee)
	require.NoError(t, err)

	members, err := svc.ListMembers(ctx, board.ID, owner)
	require.NoError(t, err)
	require.Len(t, members, 1)
}
