package card

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo  Repository
	board BoardAuthorizer
}

func NewService(repo Repository, board BoardAuthorizer) *Service {
	return &Service{repo: repo, board: board}
}

func (s *Service) CreateColumn(ctx context.Context, boardID, requesterID uuid.UUID, title string) (Column, error) {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return Column{}, err
	}
	return s.repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: title})
}

func (s *Service) RenameColumn(ctx context.Context, columnID, requesterID uuid.UUID, title string) (Column, error) {
	column, err := s.repo.GetColumnByID(ctx, columnID)
	if err != nil {
		return Column{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return Column{}, err
	}
	return s.repo.RenameColumn(ctx, columnID, title)
}

func (s *Service) DeleteColumn(ctx context.Context, columnID, requesterID uuid.UUID) error {
	column, err := s.repo.GetColumnByID(ctx, columnID)
	if err != nil {
		return mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return err
	}
	return s.repo.DeleteColumn(ctx, columnID)
}

func (s *Service) ReorderColumns(ctx context.Context, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return err
	}
	return s.repo.ReorderColumns(ctx, boardID, orderedColumnIDs)
}

type ColumnWithCards struct {
	Column
	Cards []Card
}

func (s *Service) ListBoardColumns(ctx context.Context, boardID, requesterID uuid.UUID) ([]ColumnWithCards, error) {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return nil, err
	}
	columns, err := s.repo.ListColumnsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	result := make([]ColumnWithCards, 0, len(columns))
	for _, column := range columns {
		cards, err := s.repo.ListCardsByColumn(ctx, column.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, ColumnWithCards{Column: column, Cards: cards})
	}
	return result, nil
}

func mapColumnErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrColumnNotFound
	}
	return err
}

func (s *Service) CreateCard(ctx context.Context, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
	column, err := s.repo.GetColumnByID(ctx, columnID)
	if err != nil {
		return Card{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return Card{}, err
	}
	return s.repo.CreateCard(ctx, Card{
		ID:          uuid.New(),
		ColumnID:    columnID,
		Title:       title,
		Description: description,
		AssigneeID:  assigneeID,
		DueDate:     dueDate,
	})
}

func (s *Service) UpdateCard(ctx context.Context, cardID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
	existing, err := s.repo.GetCardByID(ctx, cardID)
	if err != nil {
		return Card{}, mapCardErr(err)
	}
	column, err := s.repo.GetColumnByID(ctx, existing.ColumnID)
	if err != nil {
		return Card{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return Card{}, err
	}
	existing.Title = title
	existing.Description = description
	existing.AssigneeID = assigneeID
	existing.DueDate = dueDate
	return s.repo.UpdateCard(ctx, existing)
}

func (s *Service) DeleteCard(ctx context.Context, cardID, requesterID uuid.UUID) error {
	existing, err := s.repo.GetCardByID(ctx, cardID)
	if err != nil {
		return mapCardErr(err)
	}
	column, err := s.repo.GetColumnByID(ctx, existing.ColumnID)
	if err != nil {
		return mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return err
	}
	return s.repo.DeleteCard(ctx, cardID)
}

func (s *Service) MoveCard(ctx context.Context, cardID, requesterID, targetColumnID uuid.UUID, targetPosition int) (Card, error) {
	existing, err := s.repo.GetCardByID(ctx, cardID)
	if err != nil {
		return Card{}, mapCardErr(err)
	}
	sourceColumn, err := s.repo.GetColumnByID(ctx, existing.ColumnID)
	if err != nil {
		return Card{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, sourceColumn.BoardID, requesterID); err != nil {
		return Card{}, err
	}

	targetColumn, err := s.repo.GetColumnByID(ctx, targetColumnID)
	if err != nil {
		return Card{}, mapColumnErr(err)
	}
	if targetColumn.BoardID != sourceColumn.BoardID {
		return Card{}, ErrColumnNotFound
	}

	movingToNewColumn := targetColumnID != existing.ColumnID
	if movingToNewColumn {
		if err := s.repo.SetCardColumn(ctx, cardID, targetColumnID); err != nil {
			return Card{}, err
		}
	}

	targetCards, err := s.repo.ListCardsByColumn(ctx, targetColumnID)
	if err != nil {
		return Card{}, err
	}
	orderedTargetIDs := reorderWithInsert(targetCards, cardID, targetPosition)
	if err := s.repo.ReorderCards(ctx, targetColumnID, orderedTargetIDs); err != nil {
		return Card{}, err
	}

	if movingToNewColumn {
		sourceCards, err := s.repo.ListCardsByColumn(ctx, sourceColumn.ID)
		if err != nil {
			return Card{}, err
		}
		orderedSourceIDs := make([]uuid.UUID, 0, len(sourceCards))
		for _, c := range sourceCards {
			orderedSourceIDs = append(orderedSourceIDs, c.ID)
		}
		if err := s.repo.ReorderCards(ctx, sourceColumn.ID, orderedSourceIDs); err != nil {
			return Card{}, err
		}
	}

	return s.repo.GetCardByID(ctx, cardID)
}

func mapCardErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrCardNotFound
	}
	return err
}

// reorderWithInsert returns the full ordered card-ID list for a column
// after moving/inserting movingCardID to targetPosition (clamped to the
// valid range), removing any existing occurrence of movingCardID first.
// Pure function — no I/O — so it's cheap to unit test directly.
func reorderWithInsert(cards []Card, movingCardID uuid.UUID, targetPosition int) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(cards))
	for _, c := range cards {
		if c.ID != movingCardID {
			ids = append(ids, c.ID)
		}
	}
	if targetPosition < 0 {
		targetPosition = 0
	}
	if targetPosition > len(ids) {
		targetPosition = len(ids)
	}
	result := make([]uuid.UUID, 0, len(ids)+1)
	result = append(result, ids[:targetPosition]...)
	result = append(result, movingCardID)
	result = append(result, ids[targetPosition:]...)
	return result
}
