//go:build integration

package card

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

func createTestBoard(t *testing.T, ctx context.Context, q *gen.Queries, ownerEmail string) (boardID, ownerID uuid.UUID) {
	t.Helper()
	owner, err := q.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: ownerEmail, PasswordHash: "hashed"})
	require.NoError(t, err)
	board, err := q.CreateBoardWithOwner(ctx, gen.CreateBoardWithOwnerParams{ID: uuid.New(), Name: "Test Board", OwnerID: owner.ID})
	require.NoError(t, err)
	return board.ID, owner.ID
}

func TestPostgresRepository_CreateAndReorderColumns(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	boardID, _ := createTestBoard(t, ctx, q, "owner1@example.com")

	first, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "To Do"})
	require.NoError(t, err)
	require.Equal(t, int32(0), first.Position)

	second, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "Doing"})
	require.NoError(t, err)
	require.Equal(t, int32(1), second.Position)

	require.NoError(t, repo.ReorderColumns(ctx, boardID, []uuid.UUID{second.ID, first.ID}))

	columns, err := repo.ListColumnsByBoard(ctx, boardID)
	require.NoError(t, err)
	require.Len(t, columns, 2)
	require.Equal(t, second.ID, columns[0].ID)
	require.Equal(t, first.ID, columns[1].ID)

	_, err = repo.GetColumnByID(ctx, uuid.New())
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_CardLifecycleAndMove(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	boardID, ownerID := createTestBoard(t, ctx, q, "owner2@example.com")
	todo, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "To Do"})
	require.NoError(t, err)
	doing, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "Doing"})
	require.NoError(t, err)

	card, err := repo.CreateCard(ctx, Card{ID: uuid.New(), ColumnID: todo.ID, Title: "Write plan", AssigneeID: &ownerID})
	require.NoError(t, err)
	require.NotNil(t, card.AssigneeID)
	require.Equal(t, ownerID, *card.AssigneeID)

	require.NoError(t, repo.SetCardColumn(ctx, card.ID, doing.ID))
	require.NoError(t, repo.ReorderCards(ctx, doing.ID, []uuid.UUID{card.ID}))

	moved, err := repo.GetCardByID(ctx, card.ID)
	require.NoError(t, err)
	require.Equal(t, doing.ID, moved.ColumnID)

	require.NoError(t, repo.DeleteCard(ctx, card.ID))
	_, err = repo.GetCardByID(ctx, card.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_CreateCard_UnknownAssignee(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	boardID, _ := createTestBoard(t, ctx, q, "owner3@example.com")
	column, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "To Do"})
	require.NoError(t, err)

	unknownUser := uuid.New()
	_, err = repo.CreateCard(ctx, Card{ID: uuid.New(), ColumnID: column.ID, Title: "Write plan", AssigneeID: &unknownUser})
	require.ErrorIs(t, err, ErrAssigneeNotFound)
}

func TestPostgresRepository_UpdateCard_UnknownAssignee(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	boardID, _ := createTestBoard(t, ctx, q, "owner4@example.com")
	column, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "To Do"})
	require.NoError(t, err)

	card, err := repo.CreateCard(ctx, Card{ID: uuid.New(), ColumnID: column.ID, Title: "Write plan"})
	require.NoError(t, err)
	require.Nil(t, card.AssigneeID)

	unknownUser := uuid.New()
	_, err = repo.UpdateCard(ctx, Card{ID: card.ID, Title: card.Title, AssigneeID: &unknownUser})
	require.ErrorIs(t, err, ErrAssigneeNotFound)
}
