-- name: CreateComment :one
INSERT INTO comments (id, card_id, author_id, body)
VALUES ($1, $2, $3, $4)
RETURNING id, card_id, author_id, body, created_at, updated_at;

-- name: GetCommentByID :one
SELECT id, card_id, author_id, body, created_at, updated_at FROM comments WHERE id = $1;

-- name: UpdateComment :one
UPDATE comments SET body = $2, updated_at = now() WHERE id = $1 RETURNING id, card_id, author_id, body, created_at, updated_at;

-- name: DeleteComment :exec
DELETE FROM comments WHERE id = $1;

-- name: ListCommentsByCard :many
SELECT id, card_id, author_id, body, created_at, updated_at FROM comments
WHERE card_id = $1 AND created_at > $2
ORDER BY created_at ASC
LIMIT $3;
