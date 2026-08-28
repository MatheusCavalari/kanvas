package activity

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

type PostgresRepository struct {
	q *gen.Queries
}

func NewPostgresRepository(q *gen.Queries) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) Insert(ctx context.Context, e Entry) error {
	return r.q.InsertActivityEntry(ctx, gen.InsertActivityEntryParams{
		ID:             e.ID,
		BoardID:        e.BoardID,
		ActorID:        e.ActorID,
		Action:         e.Action,
		EntityType:     e.EntityType,
		EntityID:       e.EntityID,
		SnapshotBefore: e.SnapshotBefore,
		SnapshotAfter:  e.SnapshotAfter,
	})
}

func (r *PostgresRepository) ListByBoard(ctx context.Context, boardID uuid.UUID, cursor time.Time, limit int) ([]Entry, error) {
	rows, err := r.q.ListActivityByBoard(ctx, gen.ListActivityByBoardParams{
		BoardID:   boardID,
		CreatedAt: cursor,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, toDomainEntry(row))
	}
	return entries, nil
}

func toDomainEntry(row gen.ActivityLog) Entry {
	return Entry{
		ID:             row.ID,
		BoardID:        row.BoardID,
		ActorID:        row.ActorID,
		Action:         row.Action,
		EntityType:     row.EntityType,
		EntityID:       row.EntityID,
		SnapshotBefore: row.SnapshotBefore,
		SnapshotAfter:  row.SnapshotAfter,
		CreatedAt:      row.CreatedAt,
	}
}
