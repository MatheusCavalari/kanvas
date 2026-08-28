-- name: CreateWebhook :one
INSERT INTO webhooks (id, board_id, owner_id, url, secret, events, active)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetWebhookByID :one
SELECT * FROM webhooks WHERE id = $1;

-- name: UpdateWebhook :one
UPDATE webhooks
SET url = $2, events = $3, active = $4
WHERE id = $1
RETURNING *;

-- name: DeleteWebhook :exec
DELETE FROM webhooks WHERE id = $1;

-- name: ListWebhooksByBoard :many
SELECT * FROM webhooks WHERE board_id = $1 ORDER BY created_at ASC;

-- name: ListActiveWebhooksByBoardAndEvent :many
SELECT * FROM webhooks
WHERE board_id = $1 AND active = true AND events @> ARRAY[$2::text]
ORDER BY created_at ASC;

-- name: CreateDelivery :one
INSERT INTO webhook_deliveries (id, webhook_id, event_type, payload, status)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateDeliveryStatus :one
UPDATE webhook_deliveries
SET status = $2, attempts = attempts + 1, response_code = $3, last_attempt_at = $4
WHERE id = $1
RETURNING *;

-- name: ListDeliveriesByWebhook :many
SELECT * FROM webhook_deliveries
WHERE webhook_id = $1 AND created_at < $2
ORDER BY created_at DESC
LIMIT $3;
