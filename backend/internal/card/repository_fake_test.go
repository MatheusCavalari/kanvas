package card

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	mu      sync.Mutex
	columns map[uuid.UUID]Column
	cards   map[uuid.UUID]Card
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		columns: make(map[uuid.UUID]Column),
		cards:   make(map[uuid.UUID]Card),
	}
}

func (f *fakeRepository) CreateColumn(ctx context.Context, c Column) (Column, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	c.Position = f.nextColumnPosition(c.BoardID)
	c.CreatedAt = now
	c.UpdatedAt = now
	f.columns[c.ID] = c
	return c, nil
}

// nextColumnPosition mirrors the real Postgres query's
// COALESCE(MAX(position)+1, 0) semantics, rather than counting rows, so
// that positions stay correct (and never collide) after deletes.
func (f *fakeRepository) nextColumnPosition(boardID uuid.UUID) int32 {
	max := int32(-1)
	found := false
	for _, c := range f.columns {
		if c.BoardID == boardID && c.Position > max {
			max = c.Position
			found = true
		}
	}
	if !found {
		return 0
	}
	return max + 1
}

func (f *fakeRepository) GetColumnByID(ctx context.Context, id uuid.UUID) (Column, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.columns[id]
	if !ok {
		return Column{}, ErrNotFound
	}
	return c, nil
}

func (f *fakeRepository) RenameColumn(ctx context.Context, id uuid.UUID, title string) (Column, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.columns[id]
	if !ok {
		return Column{}, ErrNotFound
	}
	c.Title = title
	c.UpdatedAt = time.Now()
	f.columns[id] = c
	return c, nil
}

func (f *fakeRepository) DeleteColumn(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.columns, id)
	for cardID, card := range f.cards {
		if card.ColumnID == id {
			delete(f.cards, cardID)
		}
	}
	return nil
}

func (f *fakeRepository) ListColumnsByBoard(ctx context.Context, boardID uuid.UUID) ([]Column, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []Column
	for _, c := range f.columns {
		if c.BoardID == boardID {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Position != result[j].Position {
			return result[i].Position < result[j].Position
		}
		return result[i].ID.String() < result[j].ID.String()
	})
	return result, nil
}

func (f *fakeRepository) ReorderColumns(ctx context.Context, boardID uuid.UUID, orderedColumnIDs []uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, id := range orderedColumnIDs {
		c, ok := f.columns[id]
		if !ok || c.BoardID != boardID {
			continue
		}
		c.Position = int32(i)
		c.UpdatedAt = time.Now()
		f.columns[id] = c
	}
	return nil
}

func (f *fakeRepository) CreateCard(ctx context.Context, c Card) (Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	c.Position = f.nextCardPosition(c.ColumnID)
	c.CreatedAt = now
	c.UpdatedAt = now
	f.cards[c.ID] = c
	return c, nil
}

// nextCardPosition mirrors the real Postgres query's
// COALESCE(MAX(position)+1, 0) semantics, rather than counting rows, so
// that positions stay correct (and never collide) after deletes.
func (f *fakeRepository) nextCardPosition(columnID uuid.UUID) int32 {
	max := int32(-1)
	found := false
	for _, c := range f.cards {
		if c.ColumnID == columnID && c.Position > max {
			max = c.Position
			found = true
		}
	}
	if !found {
		return 0
	}
	return max + 1
}

func (f *fakeRepository) GetCardByID(ctx context.Context, id uuid.UUID) (Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.cards[id]
	if !ok {
		return Card{}, ErrNotFound
	}
	return c, nil
}

func (f *fakeRepository) UpdateCard(ctx context.Context, c Card) (Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.cards[c.ID]
	if !ok {
		return Card{}, ErrNotFound
	}
	existing.Title = c.Title
	existing.Description = c.Description
	existing.AssigneeID = c.AssigneeID
	existing.DueDate = c.DueDate
	existing.UpdatedAt = time.Now()
	f.cards[c.ID] = existing
	return existing, nil
}

func (f *fakeRepository) DeleteCard(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cards, id)
	return nil
}

func (f *fakeRepository) ListCardsByColumn(ctx context.Context, columnID uuid.UUID) ([]Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []Card
	for _, c := range f.cards {
		if c.ColumnID == columnID {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Position != result[j].Position {
			return result[i].Position < result[j].Position
		}
		return result[i].ID.String() < result[j].ID.String()
	})
	return result, nil
}

func (f *fakeRepository) SetCardColumn(ctx context.Context, cardID, columnID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.cards[cardID]
	if !ok {
		return ErrNotFound
	}
	c.ColumnID = columnID
	c.UpdatedAt = time.Now()
	f.cards[cardID] = c
	return nil
}

func (f *fakeRepository) ReorderCards(ctx context.Context, columnID uuid.UUID, orderedCardIDs []uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, id := range orderedCardIDs {
		c, ok := f.cards[id]
		if !ok || c.ColumnID != columnID {
			continue
		}
		c.Position = int32(i)
		c.UpdatedAt = time.Now()
		f.cards[id] = c
	}
	return nil
}

var errFakeNotAMember = errors.New("not a member")

type fakeBoardAuthorizer struct {
	membersByBoard map[uuid.UUID]map[uuid.UUID]bool
}

func newFakeBoardAuthorizer() *fakeBoardAuthorizer {
	return &fakeBoardAuthorizer{membersByBoard: make(map[uuid.UUID]map[uuid.UUID]bool)}
}

func (f *fakeBoardAuthorizer) addMember(boardID, userID uuid.UUID) {
	if f.membersByBoard[boardID] == nil {
		f.membersByBoard[boardID] = make(map[uuid.UUID]bool)
	}
	f.membersByBoard[boardID][userID] = true
}

func (f *fakeBoardAuthorizer) EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error {
	if f.membersByBoard[boardID] != nil && f.membersByBoard[boardID][userID] {
		return nil
	}
	return errFakeNotAMember
}

func TestFakeRepository_CreateColumnAssignsSequentialPositions(t *testing.T) {
	repo := newFakeRepository()
	ctx := context.Background()
	boardID := uuid.New()

	first, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "To Do"})
	require.NoError(t, err)
	require.Equal(t, int32(0), first.Position)

	second, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "Doing"})
	require.NoError(t, err)
	require.Equal(t, int32(1), second.Position)
}
