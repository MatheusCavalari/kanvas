package board

import (
	"context"
	"sort"
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

// CreateBoardWithOwner mirrors the real Postgres repository's atomic
// insert: the board and its owner membership row are written together
// under the same lock, so a caller never observes one without the
// other. It doesn't need real transactional rollback semantics — the
// fake never fails mid-write — just the same "both or neither" contract.
func (f *fakeRepository) CreateBoardWithOwner(ctx context.Context, b Board) (Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	b.CreatedAt = now
	b.UpdatedAt = now
	f.boards[b.ID] = b
	f.members[memberKey(b.ID, b.OwnerID)] = Member{BoardID: b.ID, UserID: b.OwnerID, Role: RoleOwner, CreatedAt: now}
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

// DeleteBoard returns nil even when the board doesn't exist, matching
// the real Postgres repository: its :exec query never checks
// CommandTag.RowsAffected(), so a delete of a nonexistent row is a
// silent no-op there too. This fake exists as the spec unit tests are
// written against, so it must not promise a not-found error production
// code will never actually produce.
func (f *fakeRepository) DeleteBoard(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.boards, id)
	for k, m := range f.members {
		if m.BoardID == id {
			delete(f.members, k)
		}
	}
	return nil
}

// ListBoardsForUser returns boards ordered by CreatedAt descending,
// matching the real query's ORDER BY b.created_at DESC.
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
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (f *fakeRepository) AddMember(ctx context.Context, m Member) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m.CreatedAt = time.Now()
	f.members[memberKey(m.BoardID, m.UserID)] = m
	return nil
}

// RemoveMember returns nil even when the target isn't a member,
// matching the real Postgres repository's :exec-based delete (see
// DeleteBoard above for why).
func (f *fakeRepository) RemoveMember(ctx context.Context, boardID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.members, memberKey(boardID, userID))
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
