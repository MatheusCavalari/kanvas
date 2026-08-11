//go:build integration

package board

import (
	"context"
	"testing"

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
