package board

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	mu      sync.Mutex
	boards  map[uuid.UUID]Board
	members map[string]Member
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		boards:  make(map[uuid.UUID]Board),
		members: make(map[string]Member),
	}
}

func memberKey(boardID, userID uuid.UUID) string {
	return boardID.String() + "|" + userID.String()
}

func (f *fakeRepository) CreateBoard(ctx context.Context, b Board) (Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	b.CreatedAt = now
	b.UpdatedAt = now
	f.boards[b.ID] = b
	return b, nil
}

func (f *fakeRepository) GetBoardByID(ctx context.Context, id uuid.UUID) (Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.boards[id]
	if !ok {
		return Board{}, ErrNotFound
	}
	return b, nil
}

func (f *fakeRepository) UpdateBoardName(ctx context.Context, id uuid.UUID, name string) (Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.boards[id]
	if !ok {
		return Board{}, ErrNotFound
	}
	b.Name = name
	b.UpdatedAt = time.Now()
	f.boards[id] = b
	return b, nil
}

func (f *fakeRepository) DeleteBoard(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.boards[id]; !ok {
		return ErrNotFound
	}
	delete(f.boards, id)
	for k, m := range f.members {
		if m.BoardID == id {
			delete(f.members, k)
		}
	}
	return nil
}

func (f *fakeRepository) ListBoardsForUser(ctx context.Context, userID uuid.UUID) ([]Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []Board
	for _, m := range f.members {
		if m.UserID == userID {
			if b, ok := f.boards[m.BoardID]; ok {
				result = append(result, b)
			}
		}
	}
	return result, nil
}

func (f *fakeRepository) AddMember(ctx context.Context, m Member) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m.CreatedAt = time.Now()
	f.members[memberKey(m.BoardID, m.UserID)] = m
	return nil
}

func (f *fakeRepository) RemoveMember(ctx context.Context, boardID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := memberKey(boardID, userID)
	if _, ok := f.members[key]; !ok {
		return ErrNotFound
	}
	delete(f.members, key)
	return nil
}

func (f *fakeRepository) GetMember(ctx context.Context, boardID, userID uuid.UUID) (Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.members[memberKey(boardID, userID)]
	if !ok {
		return Member{}, ErrNotFound
	}
	return m, nil
}

func (f *fakeRepository) ListMembers(ctx context.Context, boardID uuid.UUID) ([]Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []Member
	for _, m := range f.members {
		if m.BoardID == boardID {
			result = append(result, m)
		}
	}
	return result, nil
}

func TestFakeRepository_CreateBoardAddsNoMembers(t *testing.T) {
	repo := newFakeRepository()
	ctx := context.Background()

	owner := uuid.New()
	board, err := repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: "Sprint Board", OwnerID: owner})
	require.NoError(t, err)
	require.Equal(t, "Sprint Board", board.Name)

	_, err = repo.GetMember(ctx, board.ID, owner)
	require.ErrorIs(t, err, ErrNotFound)
}
