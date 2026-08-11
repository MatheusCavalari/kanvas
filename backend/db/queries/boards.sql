-- name: CreateBoard :one
INSERT INTO boards (id, name, owner_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetBoardByID :one
SELECT * FROM boards WHERE id = $1;

-- name: UpdateBoardName :one
UPDATE boards SET name = $2, updated_at = now() WHERE id = $1
RETURNING *;

-- name: DeleteBoard :exec
DELETE FROM boards WHERE id = $1;

-- name: ListBoardsForUser :many
SELECT b.* FROM boards b
JOIN board_members bm ON bm.board_id = b.id
WHERE bm.user_id = $1
ORDER BY b.created_at DESC;
