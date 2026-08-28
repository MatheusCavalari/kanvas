package webhook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
)

// secretBytes is the number of random bytes used to generate a webhook
// secret. Hex-encoded, this produces a 64-character string, matching the
// webhooks.secret VARCHAR(64) column.
const secretBytes = 32

type Service struct {
	repo  Repository
	owner BoardOwnerChecker
}

func NewService(repo Repository, owner BoardOwnerChecker) *Service {
	return &Service{repo: repo, owner: owner}
}

// Create registers a new webhook on boardID. Only the board owner may do
// so. The generated secret is returned in the result but is not retained
// anywhere else the caller can retrieve it later — it must be copied down
// at creation time.
func (s *Service) Create(ctx context.Context, boardID, requesterID uuid.UUID, rawURL string, events []string) (Webhook, error) {
	if !s.owner.IsBoardOwner(ctx, boardID, requesterID) {
		return Webhook{}, ErrForbidden
	}
	if err := ValidateURL(rawURL); err != nil {
		return Webhook{}, err
	}
	if err := ValidateEvents(events); err != nil {
		return Webhook{}, err
	}

	secret, err := generateSecret()
	if err != nil {
		return Webhook{}, err
	}

	w := Webhook{
		ID:      uuid.New(),
		BoardID: boardID,
		OwnerID: requesterID,
		URL:     rawURL,
		Secret:  secret,
		Events:  events,
		Active:  true,
	}
	created, err := s.repo.Create(ctx, w)
	if err != nil {
		return Webhook{}, mapErr(err)
	}
	return created, nil
}

// Update changes a webhook's URL, subscribed events, and active flag. Only
// the board owner may do so, and webhookID must belong to boardID.
func (s *Service) Update(ctx context.Context, boardID, webhookID, requesterID uuid.UUID, rawURL string, events []string, active bool) (Webhook, error) {
	if !s.owner.IsBoardOwner(ctx, boardID, requesterID) {
		return Webhook{}, ErrForbidden
	}
	existing, err := s.repo.GetByID(ctx, webhookID)
	if err != nil {
		return Webhook{}, mapErr(err)
	}
	if existing.BoardID != boardID {
		return Webhook{}, ErrWebhookNotFound
	}
	if err := ValidateURL(rawURL); err != nil {
		return Webhook{}, err
	}
	if err := ValidateEvents(events); err != nil {
		return Webhook{}, err
	}

	updated, err := s.repo.Update(ctx, webhookID, rawURL, events, active)
	if err != nil {
		return Webhook{}, mapErr(err)
	}
	return updated, nil
}

// Delete removes a webhook. Only the board owner may do so, and webhookID
// must belong to boardID.
func (s *Service) Delete(ctx context.Context, boardID, webhookID, requesterID uuid.UUID) error {
	if !s.owner.IsBoardOwner(ctx, boardID, requesterID) {
		return ErrForbidden
	}
	existing, err := s.repo.GetByID(ctx, webhookID)
	if err != nil {
		return mapErr(err)
	}
	if existing.BoardID != boardID {
		return ErrWebhookNotFound
	}
	return s.repo.Delete(ctx, webhookID)
}

// ListByBoard lists a board's webhooks. Only the board owner may do so.
func (s *Service) ListByBoard(ctx context.Context, boardID, requesterID uuid.UUID) ([]Webhook, error) {
	if !s.owner.IsBoardOwner(ctx, boardID, requesterID) {
		return nil, ErrForbidden
	}
	return s.repo.ListByBoard(ctx, boardID)
}

// ListDeliveries lists the delivery attempts for a webhook, newest first.
// Only the board owner may do so, and webhookID must belong to boardID.
func (s *Service) ListDeliveries(ctx context.Context, boardID, webhookID, requesterID uuid.UUID, cursor time.Time, limit int) ([]Delivery, error) {
	if !s.owner.IsBoardOwner(ctx, boardID, requesterID) {
		return nil, ErrForbidden
	}
	existing, err := s.repo.GetByID(ctx, webhookID)
	if err != nil {
		return nil, mapErr(err)
	}
	if existing.BoardID != boardID {
		return nil, ErrWebhookNotFound
	}

	if limit <= 0 {
		limit = DefaultDeliveriesLimit
	}
	if limit > MaxDeliveriesLimit {
		limit = MaxDeliveriesLimit
	}
	return s.repo.ListDeliveriesByWebhook(ctx, webhookID, cursor, limit)
}

func generateSecret() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func mapErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrWebhookNotFound
	}
	return err
}
