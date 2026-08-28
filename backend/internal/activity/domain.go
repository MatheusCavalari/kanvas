package activity

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Entry is one row of the audit log: who (ActorID) did what (Action) to
// which entity (EntityType/EntityID) on which board (BoardID), and when.
type Entry struct {
	ID             uuid.UUID
	BoardID        uuid.UUID
	ActorID        uuid.UUID
	Action         string
	EntityType     string
	EntityID       uuid.UUID
	SnapshotBefore []byte
	SnapshotAfter  []byte
	CreatedAt      time.Time
}

// Repository persists and lists activity log entries.
type Repository interface {
	Insert(ctx context.Context, e Entry) error
	ListByBoard(ctx context.Context, boardID uuid.UUID, cursor time.Time, limit int) ([]Entry, error)
}

// EventPublisher is the local, decoupled view of the publisher this package
// wraps and delegates to. It mirrors the EventPublisher interface defined
// by the domain packages (card, label, comment) and internal/platform/cache,
// rather than importing any of them, so this package stays decoupled.
type EventPublisher interface {
	Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{})
}

// BoardAuthorizer verifies board membership before allowing a caller to
// read the activity log for a board.
type BoardAuthorizer interface {
	EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error
}

// DefaultListLimit is used when ListByBoard is called with a non-positive
// limit.
const DefaultListLimit = 50

// MaxListLimit caps the page size ListByBoard will ever return, regardless
// of what the caller requests.
const MaxListLimit = 200
