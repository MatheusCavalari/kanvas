package board

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Service struct {
	repo  Repository
	users UserLookup
}

func NewService(repo Repository, users UserLookup) *Service {
	return &Service{repo: repo, users: users}
}

func (s *Service) CreateBoard(ctx context.Context, ownerID uuid.UUID, name string) (Board, error) {
	board, err := s.repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: name, OwnerID: ownerID})
	if err != nil {
		return Board{}, err
	}
	if err := s.repo.AddMember(ctx, Member{BoardID: board.ID, UserID: ownerID, Role: RoleOwner}); err != nil {
		return Board{}, err
	}
	return board, nil
}

func (s *Service) ListBoards(ctx context.Context, userID uuid.UUID) ([]Board, error) {
	return s.repo.ListBoardsForUser(ctx, userID)
}

func (s *Service) GetBoard(ctx context.Context, boardID, requesterID uuid.UUID) (Board, error) {
	if _, err := s.requireMember(ctx, boardID, requesterID); err != nil {
		return Board{}, err
	}
	return s.repo.GetBoardByID(ctx, boardID)
}

// requireMember returns ErrNotAMember both when the board doesn't exist
// and when it exists but the user isn't on it — deliberately not
// distinguishing the two, so a non-member can't probe for board
// existence via the error code.
func (s *Service) requireMember(ctx context.Context, boardID, userID uuid.UUID) (Member, error) {
	m, err := s.repo.GetMember(ctx, boardID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Member{}, ErrNotAMember
		}
		return Member{}, err
	}
	return m, nil
}

func (s *Service) requireOwner(ctx context.Context, boardID, userID uuid.UUID) error {
	m, err := s.requireMember(ctx, boardID, userID)
	if err != nil {
		return err
	}
	if m.Role != RoleOwner {
		return ErrForbidden
	}
	return nil
}
