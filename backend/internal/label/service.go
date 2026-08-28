package label

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	EventLabelCreated     = "label.created"
	EventLabelUpdated     = "label.updated"
	EventLabelDeleted     = "label.deleted"
	EventCardLabelAdded   = "card.label_added"
	EventCardLabelRemoved = "card.label_removed"
)

// labelEventView is the wire representation of a Label used for realtime
// event payloads. It exists because Label has no json tags, and the
// frontend expects snake_case field names (e.g. board_id, not BoardID).
type labelEventView struct {
	ID        uuid.UUID `json:"id"`
	BoardID   uuid.UUID `json:"board_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

func newLabelEventView(l Label) labelEventView {
	return labelEventView{
		ID:        l.ID,
		BoardID:   l.BoardID,
		Name:      l.Name,
		Color:     l.Color,
		CreatedAt: l.CreatedAt,
	}
}

type Service struct {
	repo   Repository
	board  BoardAuthorizer
	cards  CardLookup
	events EventPublisher
}

func NewService(repo Repository, board BoardAuthorizer, cards CardLookup, events EventPublisher) *Service {
	return &Service{repo: repo, board: board, cards: cards, events: events}
}

func (s *Service) Create(ctx context.Context, boardID, requesterID uuid.UUID, name, color string) (Label, error) {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return Label{}, err
	}
	if err := ValidateName(name); err != nil {
		return Label{}, err
	}
	if err := ValidateColor(color); err != nil {
		return Label{}, err
	}
	l, err := s.repo.Create(ctx, Label{ID: uuid.New(), BoardID: boardID, Name: name, Color: color})
	if err != nil {
		return Label{}, mapErr(err)
	}
	s.events.Publish(ctx, boardID, EventLabelCreated, newLabelEventView(l))
	return l, nil
}

func (s *Service) Update(ctx context.Context, labelID, requesterID uuid.UUID, name, color string) (Label, error) {
	existing, err := s.repo.GetByID(ctx, labelID)
	if err != nil {
		return Label{}, mapErr(err)
	}
	if err := s.board.EnsureMember(ctx, existing.BoardID, requesterID); err != nil {
		return Label{}, err
	}
	if err := ValidateName(name); err != nil {
		return Label{}, err
	}
	if err := ValidateColor(color); err != nil {
		return Label{}, err
	}
	updated, err := s.repo.Update(ctx, labelID, name, color)
	if err != nil {
		return Label{}, mapErr(err)
	}
	s.events.Publish(ctx, existing.BoardID, EventLabelUpdated, newLabelEventView(updated))
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, labelID, requesterID uuid.UUID) error {
	existing, err := s.repo.GetByID(ctx, labelID)
	if err != nil {
		return mapErr(err)
	}
	if err := s.board.EnsureMember(ctx, existing.BoardID, requesterID); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, labelID); err != nil {
		return err
	}
	s.events.Publish(ctx, existing.BoardID, EventLabelDeleted, map[string]interface{}{
		"id": labelID, "board_id": existing.BoardID,
	})
	return nil
}

func (s *Service) ListByBoard(ctx context.Context, boardID, requesterID uuid.UUID) ([]Label, error) {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return nil, err
	}
	return s.repo.ListByBoard(ctx, boardID)
}

func (s *Service) AttachToCard(ctx context.Context, cardID, labelID, requesterID uuid.UUID) error {
	lbl, err := s.repo.GetByID(ctx, labelID)
	if err != nil {
		return mapErr(err)
	}
	cardBoardID, err := s.cards.CardBoardID(ctx, cardID)
	if err != nil {
		return mapErr(err)
	}
	if cardBoardID != lbl.BoardID {
		return ErrForbidden
	}
	if err := s.board.EnsureMember(ctx, cardBoardID, requesterID); err != nil {
		return err
	}
	if err := s.repo.AttachToCard(ctx, cardID, labelID); err != nil {
		return err
	}
	s.events.Publish(ctx, lbl.BoardID, EventCardLabelAdded, map[string]interface{}{
		"card_id": cardID, "label_id": labelID,
	})
	return nil
}

func (s *Service) DetachFromCard(ctx context.Context, cardID, labelID, requesterID uuid.UUID) error {
	lbl, err := s.repo.GetByID(ctx, labelID)
	if err != nil {
		return mapErr(err)
	}
	cardBoardID, err := s.cards.CardBoardID(ctx, cardID)
	if err != nil {
		return mapErr(err)
	}
	if cardBoardID != lbl.BoardID {
		return ErrForbidden
	}
	if err := s.board.EnsureMember(ctx, cardBoardID, requesterID); err != nil {
		return err
	}
	if err := s.repo.DetachFromCard(ctx, cardID, labelID); err != nil {
		return err
	}
	s.events.Publish(ctx, lbl.BoardID, EventCardLabelRemoved, map[string]interface{}{
		"card_id": cardID, "label_id": labelID,
	})
	return nil
}

func (s *Service) ListCardLabels(ctx context.Context, cardID, requesterID uuid.UUID) ([]Label, error) {
	boardID, err := s.cards.CardBoardID(ctx, cardID)
	if err != nil {
		return nil, mapErr(err)
	}
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return nil, err
	}
	return s.repo.ListCardLabels(ctx, cardID)
}

func mapErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrLabelNotFound
	}
	return err
}
