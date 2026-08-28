package webhook

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Webhook is a board owner's subscription to a set of event types. When a
// matching event is published on the board, the Dispatcher enqueues a
// delivery job that POSTs the event payload to URL, signed with Secret.
type Webhook struct {
	ID        uuid.UUID
	BoardID   uuid.UUID
	OwnerID   uuid.UUID
	URL       string
	Secret    string
	Events    []string
	Active    bool
	CreatedAt time.Time
}

// Delivery records the outcome of one attempt to deliver an event to a
// webhook.
type Delivery struct {
	ID            uuid.UUID
	WebhookID     uuid.UUID
	EventType     string
	Payload       []byte
	Status        string
	Attempts      int
	ResponseCode  *int
	LastAttemptAt *time.Time
	CreatedAt     time.Time
}

const (
	DeliveryStatusPending = "pending"
	DeliveryStatusSuccess = "success"
	DeliveryStatusFailed  = "failed"
)

// SignatureHeader is the HTTP header carrying the hex-encoded HMAC-SHA256
// signature of the delivered request body, computed with the webhook's
// secret.
const SignatureHeader = "X-Webhook-Signature"

var (
	ErrWebhookNotFound = errors.New("webhook not found")
	ErrForbidden       = errors.New("only the board owner can manage webhooks")
	ErrInvalidURL      = errors.New("url must be an absolute http or https url")
	ErrInvalidEvents   = errors.New("at least one non-empty event type must be specified")
)

// DefaultDeliveriesLimit is used when ListDeliveries is called with a
// non-positive limit.
const DefaultDeliveriesLimit = 20

// MaxDeliveriesLimit caps the page size ListDeliveries will ever return,
// regardless of what the caller requests.
const MaxDeliveriesLimit = 100

// ValidateURL checks that raw is an absolute http(s) URL with a host, which
// is the minimum needed to POST a delivery to it.
func ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ErrInvalidURL
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ErrInvalidURL
	}
	return nil
}

// ValidateEvents checks that events is non-empty and every entry is a
// non-blank string, after trimming whitespace. It does not further
// constrain the values, since webhook packages stay decoupled from the
// event-type constants declared by other domain packages (card, label,
// comment, activity).
func ValidateEvents(events []string) error {
	if len(events) == 0 {
		return ErrInvalidEvents
	}
	for _, e := range events {
		if strings.TrimSpace(e) == "" {
			return ErrInvalidEvents
		}
	}
	return nil
}
