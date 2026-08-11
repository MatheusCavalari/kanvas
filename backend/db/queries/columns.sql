-- name: CreateColumn :one
INSERT INTO columns (id, board_id, title, position)
VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM columns WHERE board_id = $2))
RETURNING *;

-- name: GetColumnByID :one
SELECT * FROM columns WHERE id = $1;

-- name: RenameColumn :one
UPDATE columns SET title = $2, updated_at = now() WHERE id = $1
RETURNING *;

-- name: DeleteColumn :exec
DELETE FROM columns WHERE id = $1;

-- name: ListColumnsByBoard :many
SELECT * FROM columns WHERE board_id = $1 ORDER BY position ASC;

-- name: ReorderColumns :exec
UPDATE columns AS c
SET position = data.position, updated_at = now()
FROM (SELECT unnest(sqlc.arg(column_ids)::uuid[]) AS id, unnest(sqlc.arg(positions)::int[]) AS position) AS data
WHERE c.id = data.id AND c.board_id = sqlc.arg(board_id);
