//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

func TestPostgresRepository_CreateAndGetUser(t *testing.T) {
	pool := dbtest.NewPool(t)
	repo := NewPostgresRepository(gen.New(pool))
	ctx := context.Background()

	created, err := repo.CreateUser(ctx, User{
		ID:           uuid.New(),
		Name:         "Ada Lovelace",
		Email:        "ada@example.com",
		PasswordHash: "hashed",
	})
	require.NoError(t, err)
	require.Equal(t, "ada@example.com", created.Email)

	fetched, err := repo.GetUserByEmail(ctx, "ada@example.com")
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)

	_, err = repo.GetUserByEmail(ctx, "missing@example.com")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_CreateUser_DuplicateEmail(t *testing.T) {
	pool := dbtest.NewPool(t)
	repo := NewPostgresRepository(gen.New(pool))
	ctx := context.Background()

	_, err := repo.CreateUser(ctx, User{
		ID:           uuid.New(),
		Name:         "Ada Lovelace",
		Email:        "dup@example.com",
		PasswordHash: "hashed",
	})
	require.NoError(t, err)

	_, err = repo.CreateUser(ctx, User{
		ID:           uuid.New(),
		Name:         "Someone Else",
		Email:        "dup@example.com",
		PasswordHash: "hashed",
	})
	require.ErrorIs(t, err, ErrEmailTaken)
}

func TestPostgresRepository_RefreshTokenLifecycle(t *testing.T) {
	pool := dbtest.NewPool(t)
	repo := NewPostgresRepository(gen.New(pool))
	ctx := context.Background()

	user, err := repo.CreateUser(ctx, User{ID: uuid.New(), Name: "Ada", Email: "ada2@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	token := RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: "some-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, repo.CreateRefreshToken(ctx, token))

	fetched, err := repo.GetRefreshTokenByHash(ctx, "some-hash")
	require.NoError(t, err)
	require.Nil(t, fetched.RevokedAt)

	require.NoError(t, repo.RevokeRefreshToken(ctx, token.ID))

	fetched, err = repo.GetRefreshTokenByHash(ctx, "some-hash")
	require.NoError(t, err)
	require.NotNil(t, fetched.RevokedAt)
}
