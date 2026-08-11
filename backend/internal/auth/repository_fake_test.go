package auth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	mu            sync.Mutex
	usersByID     map[uuid.UUID]User
	usersByEmail  map[string]uuid.UUID
	refreshTokens map[uuid.UUID]RefreshToken
	tokensByHash  map[string]uuid.UUID
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		usersByID:     make(map[uuid.UUID]User),
		usersByEmail:  make(map[string]uuid.UUID),
		refreshTokens: make(map[uuid.UUID]RefreshToken),
		tokensByHash:  make(map[string]uuid.UUID),
	}
}

func (f *fakeRepository) CreateUser(ctx context.Context, u User) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usersByID[u.ID] = u
	f.usersByEmail[u.Email] = u.ID
	return u, nil
}

func (f *fakeRepository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.usersByEmail[email]
	if !ok {
		return User{}, ErrNotFound
	}
	return f.usersByID[id], nil
}

func (f *fakeRepository) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.usersByID[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeRepository) CreateRefreshToken(ctx context.Context, t RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshTokens[t.ID] = t
	f.tokensByHash[t.TokenHash] = t.ID
	return nil
}

func (f *fakeRepository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.tokensByHash[tokenHash]
	if !ok {
		return RefreshToken{}, ErrNotFound
	}
	return f.refreshTokens[id], nil
}

func (f *fakeRepository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.refreshTokens[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now()
	t.RevokedAt = &now
	f.refreshTokens[id] = t
	return nil
}

func TestFakeRepository_CreateAndGetUser(t *testing.T) {
	repo := newFakeRepository()
	ctx := context.Background()

	created, err := repo.CreateUser(ctx, User{ID: uuid.New(), Name: "Ada", Email: "ada@example.com"})
	require.NoError(t, err)

	fetched, err := repo.GetUserByEmail(ctx, "ada@example.com")
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)

	_, err = repo.GetUserByEmail(ctx, "missing@example.com")
	require.ErrorIs(t, err, ErrNotFound)
}
