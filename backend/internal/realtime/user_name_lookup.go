package realtime

import (
	"context"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

// UserNameLookupAdapter implements UserNameLookup by reusing the
// GetUserByID query sqlc already generated for the auth package, the same
// "reuse the generated query, don't add a new one" pattern
// board.UserLookupAdapter uses for GetUserByEmail.
type UserNameLookupAdapter struct {
	q *gen.Queries
}

func NewUserNameLookupAdapter(q *gen.Queries) *UserNameLookupAdapter {
	return &UserNameLookupAdapter{q: q}
}

func (a *UserNameLookupAdapter) GetUserName(ctx context.Context, userID uuid.UUID) (string, error) {
	user, err := a.q.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.Name, nil
}
