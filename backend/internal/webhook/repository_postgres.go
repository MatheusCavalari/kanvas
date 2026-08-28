package webhook

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

func (r *PostgresRepository) Create(ctx context.Context, w Webhook) (Webhook, error) {
	row, err := r.q.CreateWebhook(ctx, gen.CreateWebhookParams{
		ID:      w.ID,
		BoardID: w.BoardID,
		OwnerID: w.OwnerID,
		Url:     w.URL,
		Secret:  w.Secret,
		Events:  w.Events,
		Active:  w.Active,
	})
	if err != nil {
		return Webhook{}, err
	}
	return toDomainWebhook(row), nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (Webhook, error) {
	row, err := r.q.GetWebhookByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Webhook{}, ErrNotFound
		}
		return Webhook{}, err
	}
	return toDomainWebhook(row), nil
}

func (r *PostgresRepository) Update(ctx context.Context, id uuid.UUID, url string, events []string, active bool) (Webhook, error) {
	row, err := r.q.UpdateWebhook(ctx, gen.UpdateWebhookParams{
		ID:     id,
		Url:    url,
		Events: events,
		Active: active,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Webhook{}, ErrNotFound
		}
		return Webhook{}, err
	}
	return toDomainWebhook(row), nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteWebhook(ctx, id)
}

func (r *PostgresRepository) ListByBoard(ctx context.Context, boardID uuid.UUID) ([]Webhook, error) {
	rows, err := r.q.ListWebhooksByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	webhooks := make([]Webhook, 0, len(rows))
	for _, row := range rows {
		webhooks = append(webhooks, toDomainWebhook(row))
	}
	return webhooks, nil
}

func (r *PostgresRepository) ListActiveByBoardAndEvent(ctx context.Context, boardID uuid.UUID, eventType string) ([]Webhook, error) {
	rows, err := r.q.ListActiveWebhooksByBoardAndEvent(ctx, gen.ListActiveWebhooksByBoardAndEventParams{
		BoardID: boardID,
		Column2: eventType,
	})
	if err != nil {
		return nil, err
	}
	webhooks := make([]Webhook, 0, len(rows))
	for _, row := range rows {
		webhooks = append(webhooks, toDomainWebhook(row))
	}
	return webhooks, nil
}

func (r *PostgresRepository) CreateDelivery(ctx context.Context, d Delivery) (Delivery, error) {
	row, err := r.q.CreateDelivery(ctx, gen.CreateDeliveryParams{
		ID:        d.ID,
		WebhookID: d.WebhookID,
		EventType: d.EventType,
		Payload:   d.Payload,
		Status:    d.Status,
	})
	if err != nil {
		return Delivery{}, err
	}
	return toDomainDelivery(row), nil
}

func (r *PostgresRepository) UpdateDeliveryStatus(ctx context.Context, id uuid.UUID, status string, responseCode *int, lastAttemptAt time.Time) (Delivery, error) {
	var code *int32
	if responseCode != nil {
		c := int32(*responseCode)
		code = &c
	}
	row, err := r.q.UpdateDeliveryStatus(ctx, gen.UpdateDeliveryStatusParams{
		ID:            id,
		Status:        status,
		ResponseCode:  code,
		LastAttemptAt: &lastAttemptAt,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Delivery{}, ErrNotFound
		}
		return Delivery{}, err
	}
	return toDomainDelivery(row), nil
}

func (r *PostgresRepository) ListDeliveriesByWebhook(ctx context.Context, webhookID uuid.UUID, cursor time.Time, limit int) ([]Delivery, error) {
	rows, err := r.q.ListDeliveriesByWebhook(ctx, gen.ListDeliveriesByWebhookParams{
		WebhookID: webhookID,
		CreatedAt: cursor,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, err
	}
	deliveries := make([]Delivery, 0, len(rows))
	for _, row := range rows {
		deliveries = append(deliveries, toDomainDelivery(row))
	}
	return deliveries, nil
}

func toDomainWebhook(row gen.Webhook) Webhook {
	return Webhook{
		ID:        row.ID,
		BoardID:   row.BoardID,
		OwnerID:   row.OwnerID,
		URL:       row.Url,
		Secret:    row.Secret,
		Events:    row.Events,
		Active:    row.Active,
		CreatedAt: row.CreatedAt,
	}
}

func toDomainDelivery(row gen.WebhookDelivery) Delivery {
	var responseCode *int
	if row.ResponseCode != nil {
		c := int(*row.ResponseCode)
		responseCode = &c
	}
	return Delivery{
		ID:            row.ID,
		WebhookID:     row.WebhookID,
		EventType:     row.EventType,
		Payload:       row.Payload,
		Status:        row.Status,
		Attempts:      int(row.Attempts),
		ResponseCode:  responseCode,
		LastAttemptAt: row.LastAttemptAt,
		CreatedAt:     row.CreatedAt,
	}
}
