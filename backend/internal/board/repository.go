package board

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	CreateBoard(ctx context.Context, b Board) (Board, error)
	// CreateBoardWithOwner atomically inserts the board and its owner
	// membership row in a single statement, so there is no window in
	// which the board exists without an owner membership.
	CreateBoardWithOwner(ctx context.Context, b Board) (Board, error)
	GetBoardByID(ctx context.Context, id uuid.UUID) (Board, error)
	UpdateBoardName(ctx context.Context, id uuid.UUID, name string) (Board, error)
	DeleteBoard(ctx context.Context, id uuid.UUID) error
	ListBoardsForUser(ctx context.Context, userID uuid.UUID) ([]Board, error)

	AddMember(ctx context.Context, m Member) error
	RemoveMember(ctx context.Context, boardID, userID uuid.UUID) error
	GetMember(ctx context.Context, boardID, userID uuid.UUID) (Member, error)
	ListMembers(ctx context.Context, boardID uuid.UUID) ([]Member, error)
}

// UserLookup resolves an email to a user ID. It exists so the board
// package never depends on the auth package directly — main.go wires a
// concrete implementation (Task 6's UserLookupAdapter) that happens to
// reuse auth's generated GetUserByEmail query under the hood.
type UserLookup interface {
	UserIDByEmail(ctx context.Context, email string) (uuid.UUID, error)
}
