package label

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

func (r *PostgresRepository) Create(ctx context.Context, l Label) (Label, error) {
	row, err := r.q.CreateLabel(ctx, gen.CreateLabelParams{
		ID:      l.ID,
		BoardID: l.BoardID,
		Name:    l.Name,
		Color:   l.Color,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Label{}, ErrDuplicateName
		}
		return Label{}, err
	}
	return toDomainLabel(row), nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Label, error) {
	row, err := r.q.GetLabelByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Label{}, ErrNotFound
		}
		return Label{}, err
	}
	return toDomainLabel(row), nil
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, name, color string) (Label, error) {
	row, err := r.q.UpdateLabel(ctx, gen.UpdateLabelParams{ID: id, Name: name, Color: color})
	if err != nil {
		if isUniqueViolation(err) {
			return Label{}, ErrDuplicateName
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return Label{}, ErrNotFound
		}
		return Label{}, err
	}
	return toDomainLabel(row), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteLabel(ctx, id)
}

func (r *PostgresRepository) ListByBoard(ctx context.Context, boardID uuid.UUID) ([]Label, error) {
	rows, err := r.q.ListLabelsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	labels := make([]Label, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, toDomainLabel(row))
	}
	return labels, nil
}

func (r *PostgresRepository) AttachToCard(ctx context.Context, cardID, labelID uuid.UUID) error {
	return r.q.AttachLabel(ctx, gen.AttachLabelParams{CardID: cardID, LabelID: labelID})
}

func (r *PostgresRepository) DetachFromCard(ctx context.Context, cardID, labelID uuid.UUID) error {
	return r.q.DetachLabel(ctx, gen.DetachLabelParams{CardID: cardID, LabelID: labelID})
}

func (r *PostgresRepository) ListCardLabels(ctx context.Context, cardID uuid.UUID) ([]Label, error) {
	rows, err := r.q.ListLabelsByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	labels := make([]Label, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, toDomainLabel(row))
	}
	return labels, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func toDomainLabel(row gen.Label) Label {
	return Label{
		ID:        row.ID,
		BoardID:   row.BoardID,
		Name:      row.Name,
		Color:     row.Color,
		CreatedAt: row.CreatedAt,
	}
}
