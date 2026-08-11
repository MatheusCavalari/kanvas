//go:build integration

package board

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

func createTestUser(t *testing.T, ctx context.Context, q *gen.Queries, email string) uuid.UUID {
	t.Helper()
	user, err := q.CreateUser(ctx, gen.CreateUserParams{
		ID:           uuid.New(),
		Name:         "Test User",
		Email:        email,
		PasswordHash: "hashed",
	})
	require.NoError(t, err)
	return user.ID
}

func TestPostgresRepository_CreateAndGetBoard(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	owner := createTestUser(t, ctx, q, "owner@example.com")

	board, err := repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: "Sprint Board", OwnerID: owner})
	require.NoError(t, err)
	require.Equal(t, "Sprint Board", board.Name)

	fetched, err := repo.GetBoardByID(ctx, board.ID)
	require.NoError(t, err)
	require.Equal(t, board.ID, fetched.ID)

	_, err = repo.GetBoardByID(ctx, uuid.New())
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_MemberLifecycle(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	owner := createTestUser(t, ctx, q, "owner2@example.com")
	member := createTestUser(t, ctx, q, "member2@example.com")

	board, err := repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: "Team Board", OwnerID: owner})
	require.NoError(t, err)

	require.NoError(t, repo.AddMember(ctx, Member{BoardID: board.ID, UserID: owner, Role: RoleOwner}))
	require.NoError(t, repo.AddMember(ctx, Member{BoardID: board.ID, UserID: member, Role: RoleMember}))

	members, err := repo.ListMembers(ctx, board.ID)
	require.NoError(t, err)
	require.Len(t, members, 2)

	require.NoError(t, repo.RemoveMember(ctx, board.ID, member))

	_, err = repo.GetMember(ctx, board.ID, member)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_AddMember_DuplicateReturnsErrAlreadyMember(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	owner := createTestUser(t, ctx, q, "dup-owner@example.com")
	invitee := createTestUser(t, ctx, q, "dup-invitee@example.com")

	board, err := repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: "Dup Board", OwnerID: owner})
	require.NoError(t, err)

	require.NoError(t, repo.AddMember(ctx, Member{BoardID: board.ID, UserID: invitee, Role: RoleMember}))

	// A second AddMember for the same (board_id, user_id) violates the
	// board_members composite primary key; the repository must translate
	// that unique-violation into ErrAlreadyMember rather than surfacing
	// the raw pgx error.
	err = repo.AddMember(ctx, Member{BoardID: board.ID, UserID: invitee, Role: RoleMember})
	require.ErrorIs(t, err, ErrAlreadyMember)
}

func TestPostgresRepository_CreateBoardWithOwner_Atomic(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	owner := createTestUser(t, ctx, q, "atomic-owner@example.com")

	board, err := repo.CreateBoardWithOwner(ctx, Board{ID: uuid.New(), Name: "Atomic Board", OwnerID: owner})
	require.NoError(t, err)
	require.Equal(t, "Atomic Board", board.Name)

	member, err := repo.GetMember(ctx, board.ID, owner)
	require.NoError(t, err)
	require.Equal(t, RoleOwner, member.Role)
}

func TestPostgresRepository_ListBoardsForUser_ScopedPerUser(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	userA := createTestUser(t, ctx, q, "list-a@example.com")
	userB := createTestUser(t, ctx, q, "list-b@example.com")

	boardA, err := repo.CreateBoardWithOwner(ctx, Board{ID: uuid.New(), Name: "Board A", OwnerID: userA})
	require.NoError(t, err)
	boardB, err := repo.CreateBoardWithOwner(ctx, Board{ID: uuid.New(), Name: "Board B", OwnerID: userB})
	require.NoError(t, err)

	boardsForA, err := repo.ListBoardsForUser(ctx, userA)
	require.NoError(t, err)
	require.Len(t, boardsForA, 1)
	require.Equal(t, boardA.ID, boardsForA[0].ID)

	boardsForB, err := repo.ListBoardsForUser(ctx, userB)
	require.NoError(t, err)
	require.Len(t, boardsForB, 1)
	require.Equal(t, boardB.ID, boardsForB[0].ID)
}

func TestPostgresRepository_UpdateBoardName_PersistsChange(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	owner := createTestUser(t, ctx, q, "rename-owner@example.com")
	board, err := repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: "Old Name", OwnerID: owner})
	require.NoError(t, err)

	time.Sleep(time.Millisecond)
	updated, err := repo.UpdateBoardName(ctx, board.ID, "New Name")
	require.NoError(t, err)
	require.Equal(t, "New Name", updated.Name)
	require.True(t, updated.UpdatedAt.After(board.UpdatedAt))

	fetched, err := repo.GetBoardByID(ctx, board.ID)
	require.NoError(t, err)
	require.Equal(t, "New Name", fetched.Name)
	require.True(t, fetched.UpdatedAt.After(board.UpdatedAt))
}

func TestUserLookupAdapter_UserIDByEmail(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	adapter := NewUserLookupAdapter(q)
	ctx := context.Background()

	userID := createTestUser(t, ctx, q, "lookup@example.com")

	found, err := adapter.UserIDByEmail(ctx, "lookup@example.com")
	require.NoError(t, err)
	require.Equal(t, userID, found)

	_, err = adapter.UserIDByEmail(ctx, "missing@example.com")
	require.ErrorIs(t, err, ErrMemberUserNotFound)
}
