package card

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	CreateColumn(ctx context.Context, c Column) (Column, error)
	GetColumnByID(ctx context.Context, id uuid.UUID) (Column, error)
	RenameColumn(ctx context.Context, id uuid.UUID, title string) (Column, error)
	DeleteColumn(ctx context.Context, id uuid.UUID) error
	ListColumnsByBoard(ctx context.Context, boardID uuid.UUID) ([]Column, error)
	ReorderColumns(ctx context.Context, boardID uuid.UUID, orderedColumnIDs []uuid.UUID) error

	CreateCard(ctx context.Context, c Card) (Card, error)
	GetCardByID(ctx context.Context, id uuid.UUID) (Card, error)
	UpdateCard(ctx context.Context, c Card) (Card, error)
	DeleteCard(ctx context.Context, id uuid.UUID) error
	ListCardsByColumn(ctx context.Context, columnID uuid.UUID) ([]Card, error)
	SetCardColumn(ctx context.Context, cardID, columnID uuid.UUID) error
	ReorderCards(ctx context.Context, columnID uuid.UUID, orderedCardIDs []uuid.UUID) error
}

// BoardAuthorizer checks board membership. Implemented by *board.Service
// via its EnsureMember method — card's service depends on this interface,
// never on the board package directly.
type BoardAuthorizer interface {
	EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error
}
