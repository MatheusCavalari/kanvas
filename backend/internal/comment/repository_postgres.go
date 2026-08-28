package comment

import (
	"context"
	"errors"
	"time"

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

func (r *PostgresRepository) Create(ctx context.Context, c Comment) (Comment, error) {
	row, err := r.q.CreateComment(ctx, gen.CreateCommentParams{
		ID:       c.ID,
		CardID:   c.CardID,
		AuthorID: c.AuthorID,
		Body:     c.Body,
	})
	if err != nil {
		return Comment{}, err
	}
	return toDomainComment(row), nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Comment, error) {
	row, err := r.q.GetCommentByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Comment{}, ErrNotFound
		}
		return Comment{}, err
	}
	return toDomainComment(gen.CreateCommentRow(row)), nil
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, body string) (Comment, error) {
	row, err := r.q.UpdateComment(ctx, gen.UpdateCommentParams{ID: id, Body: body})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Comment{}, ErrNotFound
		}
		return Comment{}, err
	}
	return toDomainComment(gen.CreateCommentRow(row)), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteComment(ctx, id)
}

func (r *PostgresRepository) ListByCard(ctx context.Context, cardID uuid.UUID, cursor time.Time, limit int) ([]Comment, error) {
	rows, err := r.q.ListCommentsByCard(ctx, gen.ListCommentsByCardParams{
		CardID:    cardID,
		CreatedAt: cursor,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, err
	}
	comments := make([]Comment, 0, len(rows))
	for _, row := range rows {
		comments = append(comments, toDomainComment(gen.CreateCommentRow(row)))
	}
	return comments, nil
}

// CardBoardID resolves a card to its board by looking up the card's column
// and then that column's board. It implements CardLookup.
func (r *PostgresRepository) CardBoardID(ctx context.Context, cardID uuid.UUID) (uuid.UUID, error) {
	card, err := r.q.GetCardByID(ctx, cardID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrCardNotFound
		}
		return uuid.Nil, err
	}
	col, err := r.q.GetColumnByID(ctx, card.ColumnID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrCardNotFound
		}
		return uuid.Nil, err
	}
	return col.BoardID, nil
}

func toDomainComment(row gen.CreateCommentRow) Comment {
	return Comment{
		ID:        row.ID,
		CardID:    row.CardID,
		AuthorID:  row.AuthorID,
		Body:      row.Body,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
