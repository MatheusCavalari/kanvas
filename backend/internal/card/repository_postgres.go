package card

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

type PostgresRepository struct {
	q *gen.Queries
}

func NewPostgresRepository(q *gen.Queries) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) CreateColumn(ctx context.Context, c Column) (Column, error) {
	row, err := r.q.CreateColumn(ctx, gen.CreateColumnParams{ID: c.ID, BoardID: c.BoardID, Title: c.Title})
	if err != nil {
		return Column{}, err
	}
	return toDomainColumn(row), nil
}

func (r *PostgresRepository) GetColumnByID(ctx context.Context, id uuid.UUID) (Column, error) {
	row, err := r.q.GetColumnByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Column{}, ErrNotFound
		}
		return Column{}, err
	}
	return toDomainColumn(row), nil
}

func (r *PostgresRepository) RenameColumn(ctx context.Context, id uuid.UUID, title string) (Column, error) {
	row, err := r.q.RenameColumn(ctx, gen.RenameColumnParams{ID: id, Title: title})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Column{}, ErrNotFound
		}
		return Column{}, err
	}
	return toDomainColumn(row), nil
}

func (r *PostgresRepository) DeleteColumn(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteColumn(ctx, id)
}

func (r *PostgresRepository) ListColumnsByBoard(ctx context.Context, boardID uuid.UUID) ([]Column, error) {
	rows, err := r.q.ListColumnsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	columns := make([]Column, 0, len(rows))
	for _, row := range rows {
		columns = append(columns, toDomainColumn(row))
	}
	return columns, nil
}

func (r *PostgresRepository) ReorderColumns(ctx context.Context, boardID uuid.UUID, orderedColumnIDs []uuid.UUID) error {
	positions := make([]int32, len(orderedColumnIDs))
	for i := range orderedColumnIDs {
		positions[i] = int32(i)
	}
	return r.q.ReorderColumns(ctx, gen.ReorderColumnsParams{
		ColumnIds: orderedColumnIDs,
		Positions: positions,
		BoardID:   boardID,
	})
}

func (r *PostgresRepository) CreateCard(ctx context.Context, c Card) (Card, error) {
	row, err := r.q.CreateCard(ctx, gen.CreateCardParams{
		ID:          c.ID,
		ColumnID:    c.ColumnID,
		Title:       c.Title,
		Description: c.Description,
		AssigneeID:  c.AssigneeID,
		DueDate:     c.DueDate,
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return Card{}, ErrAssigneeNotFound
		}
		return Card{}, err
	}
	return toDomainCard(row), nil
}

func (r *PostgresRepository) GetCardByID(ctx context.Context, id uuid.UUID) (Card, error) {
	row, err := r.q.GetCardByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Card{}, ErrNotFound
		}
		return Card{}, err
	}
	return toDomainCard(row), nil
}

func (r *PostgresRepository) UpdateCard(ctx context.Context, c Card) (Card, error) {
	row, err := r.q.UpdateCard(ctx, gen.UpdateCardParams{
		ID:          c.ID,
		Title:       c.Title,
		Description: c.Description,
		AssigneeID:  c.AssigneeID,
		DueDate:     c.DueDate,
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return Card{}, ErrAssigneeNotFound
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return Card{}, ErrNotFound
		}
		return Card{}, err
	}
	return toDomainCard(row), nil
}

func (r *PostgresRepository) DeleteCard(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteCard(ctx, id)
}

func (r *PostgresRepository) ListCardsByColumn(ctx context.Context, columnID uuid.UUID) ([]Card, error) {
	rows, err := r.q.ListCardsByColumn(ctx, columnID)
	if err != nil {
		return nil, err
	}
	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, toDomainCard(row))
	}
	return cards, nil
}

func (r *PostgresRepository) SetCardColumn(ctx context.Context, cardID, columnID uuid.UUID) error {
	return r.q.SetCardColumn(ctx, gen.SetCardColumnParams{ID: cardID, ColumnID: columnID})
}

func (r *PostgresRepository) ReorderCards(ctx context.Context, columnID uuid.UUID, orderedCardIDs []uuid.UUID) error {
	positions := make([]int32, len(orderedCardIDs))
	for i := range orderedCardIDs {
		positions[i] = int32(i)
	}
	return r.q.ReorderCards(ctx, gen.ReorderCardsParams{
		CardIds:   orderedCardIDs,
		Positions: positions,
		ColumnID:  columnID,
	})
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func toDomainColumn(row gen.Column) Column {
	return Column{
		ID:        row.ID,
		BoardID:   row.BoardID,
		Title:     row.Title,
		Position:  row.Position,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toDomainCard(row gen.Card) Card {
	return Card{
		ID:          row.ID,
		ColumnID:    row.ColumnID,
		Title:       row.Title,
		Description: row.Description,
		Position:    row.Position,
		AssigneeID:  row.AssigneeID,
		DueDate:     row.DueDate,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
