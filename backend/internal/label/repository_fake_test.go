package label_test

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/label"
)

type fakeRepo struct {
	mu         sync.Mutex
	labels     map[uuid.UUID]label.Label
	cardLabels map[uuid.UUID][]uuid.UUID // cardID -> []labelID
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{labels: map[uuid.UUID]label.Label{}, cardLabels: map[uuid.UUID][]uuid.UUID{}}
}

func (f *fakeRepo) Create(_ context.Context, l label.Label) (label.Label, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.labels {
		if existing.BoardID == l.BoardID && existing.Name == l.Name {
			return label.Label{}, label.ErrDuplicateName
		}
	}
	f.labels[l.ID] = l
	return l, nil
}

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (label.Label, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.labels[id]
	if !ok {
		return label.Label{}, label.ErrNotFound
	}
	return l, nil
}

func (f *fakeRepo) Update(_ context.Context, id uuid.UUID, name, color string) (label.Label, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.labels[id]
	if !ok {
		return label.Label{}, label.ErrNotFound
	}
	l.Name = name
	l.Color = color
	f.labels[id] = l
	return l, nil
}

func (f *fakeRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.labels, id)
	return nil
}

func (f *fakeRepo) ListByBoard(_ context.Context, boardID uuid.UUID) ([]label.Label, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []label.Label
	for _, l := range f.labels {
		if l.BoardID == boardID {
			result = append(result, l)
		}
	}
	return result, nil
}

func (f *fakeRepo) AttachToCard(_ context.Context, cardID, labelID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cardLabels[cardID] = append(f.cardLabels[cardID], labelID)
	return nil
}

func (f *fakeRepo) DetachFromCard(_ context.Context, cardID, labelID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := f.cardLabels[cardID]
	for i, id := range ids {
		if id == labelID {
			f.cardLabels[cardID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	return nil
}

func (f *fakeRepo) ListCardLabels(_ context.Context, cardID uuid.UUID) ([]label.Label, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []label.Label
	for _, lid := range f.cardLabels[cardID] {
		if l, ok := f.labels[lid]; ok {
			result = append(result, l)
		}
	}
	return result, nil
}

type fakeBoardAuth struct{ allowed map[uuid.UUID]bool }

func (f *fakeBoardAuth) EnsureMember(_ context.Context, boardID, userID uuid.UUID) error {
	if f.allowed[userID] {
		return nil
	}
	return label.ErrLabelNotFound
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
		return uuid.Nil, label.ErrCardNotFound
	}
	return boardID, nil
}

type fakeEventPublisher struct {
	events []string
}

func (f *fakeEventPublisher) Publish(_ context.Context, _ uuid.UUID, eventType string, _ interface{}) {
	f.events = append(f.events, eventType)
}
