package label

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

const (
	EventLabelCreated     = "label.created"
	EventLabelUpdated     = "label.updated"
	EventLabelDeleted     = "label.deleted"
	EventCardLabelAdded   = "card.label_added"
	EventCardLabelRemoved = "card.label_removed"
)

type Service struct {
	repo   Repository
	board  BoardAuthorizer
	events EventPublisher
}

func NewService(repo Repository, board BoardAuthorizer, events EventPublisher) *Service {
	return &Service{repo: repo, board: board, events: events}
}

func (s *Service) Create(ctx context.Context, boardID, requesterID uuid.UUID, name, color string) (Label, error) {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return Label{}, err
	}
	if err := ValidateColor(color); err != nil {
		return Label{}, err
	}
	l, err := s.repo.Create(ctx, Label{ID: uuid.New(), BoardID: boardID, Name: name, Color: color})
	if err != nil {
		return Label{}, mapErr(err)
	}
	s.events.Publish(ctx, boardID, EventLabelCreated, l)
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
	if err := ValidateColor(color); err != nil {
		return Label{}, err
	}
	updated, err := s.repo.Update(ctx, labelID, name, color)
	if err != nil {
		return Label{}, mapErr(err)
	}
	s.events.Publish(ctx, existing.BoardID, EventLabelUpdated, updated)
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
	if err := s.board.EnsureMember(ctx, lbl.BoardID, requesterID); err != nil {
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
	if err := s.board.EnsureMember(ctx, lbl.BoardID, requesterID); err != nil {
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

func (s *Service) ListCardLabels(ctx context.Context, cardID uuid.UUID) ([]Label, error) {
	return s.repo.ListCardLabels(ctx, cardID)
}

func mapErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrLabelNotFound
	}
	return err
}
