package comment

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	EventCommentCreated = "comment.created"
	EventCommentUpdated = "comment.updated"
	EventCommentDeleted = "comment.deleted"
)

// DefaultListLimit is used when ListByCard is called with a non-positive
// limit.
const DefaultListLimit = 20

// MaxListLimit caps the page size ListByCard will ever return, regardless
// of what the caller requests.
const MaxListLimit = 100

// commentEventView is the wire representation of a Comment used for
// realtime event payloads. It exists because Comment has no json tags, and
// the frontend expects snake_case field names (e.g. card_id, not CardID).
type commentEventView struct {
	ID        uuid.UUID `json:"id"`
	CardID    uuid.UUID `json:"card_id"`
	AuthorID  uuid.UUID `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newCommentEventView(c Comment) commentEventView {
	return commentEventView(c)
}

type Service struct {
	repo   Repository
	cards  CardLookup
	board  BoardAuthorizer
	owner  BoardOwnerChecker
	events EventPublisher
}

// NewService wires the comment service. board authorizes membership for
// read/write access, and owner (typically the same underlying
// board.Service) additionally allows a board owner to delete any comment
// on their board, not just their own.
func NewService(repo Repository, cards CardLookup, board BoardAuthorizer, owner BoardOwnerChecker, events EventPublisher) *Service {
	return &Service{repo: repo, cards: cards, board: board, owner: owner, events: events}
}

func (s *Service) Create(ctx context.Context, cardID, authorID uuid.UUID, body string) (Comment, error) {
	boardID, err := s.cards.CardBoardID(ctx, cardID)
	if err != nil {
		return Comment{}, mapErr(err)
	}
	if err := s.board.EnsureMember(ctx, boardID, authorID); err != nil {
		return Comment{}, err
	}
	if err := ValidateBody(body); err != nil {
		return Comment{}, err
	}

	c, err := s.repo.Create(ctx, Comment{ID: uuid.New(), CardID: cardID, AuthorID: authorID, Body: body})
	if err != nil {
		return Comment{}, mapErr(err)
	}
	s.events.Publish(ctx, boardID, EventCommentCreated, newCommentEventView(c))
	return c, nil
}

func (s *Service) Update(ctx context.Context, commentID, requesterID uuid.UUID, body string) (Comment, error) {
	existing, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		return Comment{}, mapErr(err)
	}
	boardID, err := s.cards.CardBoardID(ctx, existing.CardID)
	if err != nil {
		return Comment{}, mapErr(err)
	}
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return Comment{}, err
	}
	if existing.AuthorID != requesterID {
		return Comment{}, ErrNotAuthor
	}
	if err := ValidateBody(body); err != nil {
		return Comment{}, err
	}

	updated, err := s.repo.Update(ctx, commentID, body)
	if err != nil {
		return Comment{}, mapErr(err)
	}
	s.events.Publish(ctx, boardID, EventCommentUpdated, newCommentEventView(updated))
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, commentID, requesterID uuid.UUID) error {
	existing, err := s.repo.GetByID(ctx, commentID)
	if err != nil {
		return mapErr(err)
	}
	boardID, err := s.cards.CardBoardID(ctx, existing.CardID)
	if err != nil {
		return mapErr(err)
	}
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return err
	}
	if existing.AuthorID != requesterID && !s.owner.IsBoardOwner(ctx, boardID, requesterID) {
		return ErrForbidden
	}

	if err := s.repo.Delete(ctx, commentID); err != nil {
		return err
	}
	s.events.Publish(ctx, boardID, EventCommentDeleted, map[string]interface{}{
		"id": commentID, "card_id": existing.CardID, "board_id": boardID,
	})
	return nil
}

// ListByCard returns comments for cardID created strictly after cursor,
// oldest first, capped at limit (DefaultListLimit if limit <= 0,
// MaxListLimit at most).
func (s *Service) ListByCard(ctx context.Context, cardID, requesterID uuid.UUID, cursor time.Time, limit int) ([]Comment, error) {
	boardID, err := s.cards.CardBoardID(ctx, cardID)
	if err != nil {
		return nil, mapErr(err)
	}
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = DefaultListLimit
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}
	return s.repo.ListByCard(ctx, cardID, cursor, limit)
}

func mapErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrCommentNotFound
	}
	return err
}
