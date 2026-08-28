package comment

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	Create(ctx context.Context, c Comment) (Comment, error)
	GetByID(ctx context.Context, id uuid.UUID) (Comment, error)
	Update(ctx context.Context, id uuid.UUID, body string) (Comment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListByCard(ctx context.Context, cardID uuid.UUID, cursor time.Time, limit int) ([]Comment, error)
}

// CardLookup resolves a card to the board it belongs to, so the service can
// verify board membership before allowing comment operations on the card.
type CardLookup interface {
	CardBoardID(ctx context.Context, cardID uuid.UUID) (uuid.UUID, error)
}

type BoardAuthorizer interface {
	EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error
}

// BoardOwnerChecker reports whether userID is the owner of boardID. It is
// used to allow board owners to delete any comment, in addition to the
// comment's author.
type BoardOwnerChecker interface {
	IsBoardOwner(ctx context.Context, boardID, userID uuid.UUID) bool
}

type EventPublisher interface {
	Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{})
}
