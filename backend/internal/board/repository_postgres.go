package board

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

type PostgresRepository struct {
	q *gen.Queries
}

func NewPostgresRepository(q *gen.Queries) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) CreateBoard(ctx context.Context, b Board) (Board, error) {
	row, err := r.q.CreateBoard(ctx, gen.CreateBoardParams{ID: b.ID, Name: b.Name, OwnerID: b.OwnerID})
	if err != nil {
		return Board{}, err
	}
	return toDomainBoard(row), nil
}

func (r *PostgresRepository) GetBoardByID(ctx context.Context, id uuid.UUID) (Board, error) {
	row, err := r.q.GetBoardByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Board{}, ErrNotFound
		}
		return Board{}, err
	}
	return toDomainBoard(row), nil
}

func (r *PostgresRepository) UpdateBoardName(ctx context.Context, id uuid.UUID, name string) (Board, error) {
	row, err := r.q.UpdateBoardName(ctx, gen.UpdateBoardNameParams{ID: id, Name: name})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Board{}, ErrNotFound
		}
		return Board{}, err
	}
	return toDomainBoard(row), nil
}

func (r *PostgresRepository) DeleteBoard(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteBoard(ctx, id)
}

func (r *PostgresRepository) ListBoardsForUser(ctx context.Context, userID uuid.UUID) ([]Board, error) {
	rows, err := r.q.ListBoardsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	boards := make([]Board, 0, len(rows))
	for _, row := range rows {
		boards = append(boards, toDomainBoard(row))
	}
	return boards, nil
}

func (r *PostgresRepository) AddMember(ctx context.Context, m Member) error {
	return r.q.AddBoardMember(ctx, gen.AddBoardMemberParams{BoardID: m.BoardID, UserID: m.UserID, Role: string(m.Role)})
}

func (r *PostgresRepository) RemoveMember(ctx context.Context, boardID, userID uuid.UUID) error {
	return r.q.RemoveBoardMember(ctx, gen.RemoveBoardMemberParams{BoardID: boardID, UserID: userID})
}

func (r *PostgresRepository) GetMember(ctx context.Context, boardID, userID uuid.UUID) (Member, error) {
	row, err := r.q.GetBoardMember(ctx, gen.GetBoardMemberParams{BoardID: boardID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Member{}, ErrNotFound
		}
		return Member{}, err
	}
	return toDomainMember(row), nil
}

func (r *PostgresRepository) ListMembers(ctx context.Context, boardID uuid.UUID) ([]Member, error) {
	rows, err := r.q.ListBoardMembers(ctx, boardID)
	if err != nil {
		return nil, err
	}
	members := make([]Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, toDomainMember(row))
	}
	return members, nil
}

func toDomainBoard(row gen.Board) Board {
	return Board{
		ID:        row.ID,
		Name:      row.Name,
		OwnerID:   row.OwnerID,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toDomainMember(row gen.BoardMember) Member {
	return Member{
		BoardID:   row.BoardID,
		UserID:    row.UserID,
		Role:      Role(row.Role),
		CreatedAt: row.CreatedAt,
	}
}

// UserLookupAdapter implements board.UserLookup by reusing the
// GetUserByEmail query sqlc already generated for the auth package
// (Phase 1, Task 6). This lets board resolve invite emails to user IDs
// without importing the auth package at all.
type UserLookupAdapter struct {
	q *gen.Queries
}

func NewUserLookupAdapter(q *gen.Queries) *UserLookupAdapter {
	return &UserLookupAdapter{q: q}
}

func (a *UserLookupAdapter) UserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	row, err := a.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, ErrMemberUserNotFound
		}
		return uuid.UUID{}, err
	}
	return row.ID, nil
}
