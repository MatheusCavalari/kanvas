package card

import (
	"context"
	"errors"

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
