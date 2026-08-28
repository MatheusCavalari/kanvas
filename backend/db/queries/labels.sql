-- name: CreateLabel :one
INSERT INTO labels (id, board_id, name, color)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetLabelByID :one
SELECT * FROM labels WHERE id = $1;

-- name: UpdateLabel :one
UPDATE labels SET name = $2, color = $3 WHERE id = $1 RETURNING *;

-- name: DeleteLabel :exec
DELETE FROM labels WHERE id = $1;

-- name: ListLabelsByBoard :many
SELECT * FROM labels WHERE board_id = $1 ORDER BY name ASC;
