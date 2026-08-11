-- name: CreateCard :one
INSERT INTO cards (id, column_id, title, description, position, assignee_id, due_date)
VALUES ($1, $2, $3, $4, (SELECT COALESCE(MAX(position) + 1, 0) FROM cards WHERE column_id = $2), $5, $6)
RETURNING *;

-- name: GetCardByID :one
SELECT * FROM cards WHERE id = $1;

-- name: UpdateCard :one
UPDATE cards
SET title = $2, description = $3, assignee_id = $4, due_date = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteCard :exec
DELETE FROM cards WHERE id = $1;

-- name: ListCardsByColumn :many
SELECT * FROM cards WHERE column_id = $1 ORDER BY position ASC;

-- name: SetCardColumn :exec
UPDATE cards SET column_id = $2, updated_at = now() WHERE id = $1;

-- name: ReorderCards :exec
UPDATE cards AS c
SET position = data.position, updated_at = now()
FROM (SELECT unnest(sqlc.arg(card_ids)::uuid[]) AS id, unnest(sqlc.arg(positions)::int[]) AS position) AS data
WHERE c.id = data.id AND c.column_id = sqlc.arg(column_id);
