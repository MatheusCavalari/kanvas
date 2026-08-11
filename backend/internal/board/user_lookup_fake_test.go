package board

import (
	"context"

	"github.com/google/uuid"
)

type fakeUserLookup struct {
	usersByEmail map[string]uuid.UUID
}

func newFakeUserLookup() *fakeUserLookup {
	return &fakeUserLookup{usersByEmail: make(map[string]uuid.UUID)}
}

func (f *fakeUserLookup) UserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	id, ok := f.usersByEmail[email]
	if !ok {
		return uuid.UUID{}, ErrMemberUserNotFound
	}
	return id, nil
}
