package webhook

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is the repository-level not-found error, mapped by the
// service to the domain-level ErrWebhookNotFound.
var ErrNotFound = errors.New("not found")

// Repository persists webhooks and their delivery attempts.
type Repository interface {
	Create(ctx context.Context, w Webhook) (Webhook, error)
	GetByID(ctx context.Context, id uuid.UUID) (Webhook, error)
	Update(ctx context.Context, id uuid.UUID, url string, events []string, active bool) (Webhook, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListByBoard(ctx context.Context, boardID uuid.UUID) ([]Webhook, error)

	// ListActiveByBoardAndEvent returns the active webhooks on boardID that
	// subscribe to eventType, used by the Dispatcher to find delivery
	// targets for a published event.
	ListActiveByBoardAndEvent(ctx context.Context, boardID uuid.UUID, eventType string) ([]Webhook, error)

	CreateDelivery(ctx context.Context, d Delivery) (Delivery, error)
	UpdateDeliveryStatus(ctx context.Context, id uuid.UUID, status string, responseCode *int, lastAttemptAt time.Time) (Delivery, error)
	ListDeliveriesByWebhook(ctx context.Context, webhookID uuid.UUID, cursor time.Time, limit int) ([]Delivery, error)
}

// BoardOwnerChecker reports whether userID is the owner of boardID. Webhook
// management (create/edit/delete, plus reading webhooks and their
// deliveries, since a webhook's secret and delivery history are sensitive)
// is restricted to the board owner, not just any member.
type BoardOwnerChecker interface {
	IsBoardOwner(ctx context.Context, boardID, userID uuid.UUID) bool
}

// EventPublisher is the local, decoupled view of the publisher chain this
// package both consumes (as a Dispatcher decorator) and depends on. It
// mirrors the EventPublisher interface defined by the other domain
// packages and internal/platform/cache, rather than importing any of them.
type EventPublisher interface {
	Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{})
}
