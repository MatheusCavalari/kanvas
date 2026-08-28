package comment_test

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/comment"
)

type fakeRepo struct {
	mu       sync.Mutex
	comments map[uuid.UUID]comment.Comment
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{comments: map[uuid.UUID]comment.Comment{}}
}

func (f *fakeRepo) Create(_ context.Context, c comment.Comment) (comment.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = c.CreatedAt
	f.comments[c.ID] = c
	return c, nil
}

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (comment.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.comments[id]
	if !ok {
		return comment.Comment{}, comment.ErrNotFound
	}
	return c, nil
}

func (f *fakeRepo) Update(_ context.Context, id uuid.UUID, body string) (comment.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.comments[id]
	if !ok {
		return comment.Comment{}, comment.ErrNotFound
	}
	c.Body = body
	c.UpdatedAt = time.Now()
	f.comments[id] = c
	return c, nil
}

func (f *fakeRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.comments, id)
	return nil
}

func (f *fakeRepo) ListByCard(_ context.Context, cardID uuid.UUID, cursor time.Time, limit int) ([]comment.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []comment.Comment
	for _, c := range f.comments {
		if c.CardID == cardID && c.CreatedAt.After(cursor) {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

type fakeBoardAuth struct{ allowed map[uuid.UUID]bool }

func (f *fakeBoardAuth) EnsureMember(_ context.Context, _, userID uuid.UUID) error {
	if f.allowed[userID] {
		return nil
	}
	return comment.ErrCommentNotFound
}

type fakeOwnerChecker struct{ owners map[uuid.UUID]bool }

func newFakeOwnerChecker() *fakeOwnerChecker {
	return &fakeOwnerChecker{owners: map[uuid.UUID]bool{}}
}

func (f *fakeOwnerChecker) IsBoardOwner(_ context.Context, _, userID uuid.UUID) bool {
	return f.owners[userID]
}

// fakeCardLookup maps a card ID to the board it belongs to, mirroring the
// production CardBoardID adapter that resolves through columns.
type fakeCardLookup struct {
	boards map[uuid.UUID]uuid.UUID // cardID -> boardID
}

func newFakeCardLookup() *fakeCardLookup {
	return &fakeCardLookup{boards: map[uuid.UUID]uuid.UUID{}}
}

func (f *fakeCardLookup) CardBoardID(_ context.Context, cardID uuid.UUID) (uuid.UUID, error) {
	boardID, ok := f.boards[cardID]
	if !ok {
		return uuid.Nil, comment.ErrCardNotFound
	}
	return boardID, nil
}

type fakeEventPublisher struct {
	mu     sync.Mutex
	events []string
}

func (f *fakeEventPublisher) Publish(_ context.Context, _ uuid.UUID, eventType string, _ interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, eventType)
}

func (f *fakeEventPublisher) has(eventType string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.events {
		if e == eventType {
			return true
		}
	}
	return false
}
