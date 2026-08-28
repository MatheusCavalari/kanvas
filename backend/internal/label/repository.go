package label

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	Create(ctx context.Context, l Label) (Label, error)
	GetByID(ctx context.Context, id uuid.UUID) (Label, error)
	Update(ctx context.Context, id uuid.UUID, name, color string) (Label, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListByBoard(ctx context.Context, boardID uuid.UUID) ([]Label, error)
	AttachToCard(ctx context.Context, cardID, labelID uuid.UUID) error
	DetachFromCard(ctx context.Context, cardID, labelID uuid.UUID) error
	ListCardLabels(ctx context.Context, cardID uuid.UUID) ([]Label, error)
}

type BoardAuthorizer interface {
	EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error
}

// CardLookup resolves a card to the board it belongs to, so the service can
// verify a card is in the same board as the label being attached/detached,
// and authorize the requester against that board.
type CardLookup interface {
	CardBoardID(ctx context.Context, cardID uuid.UUID) (uuid.UUID, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{})
}
