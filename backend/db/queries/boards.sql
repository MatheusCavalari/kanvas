-- name: CreateBoard :one
INSERT INTO boards (id, name, owner_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CreateBoardWithOwner :one
WITH new_board AS (
    INSERT INTO boards (id, name, owner_id) VALUES ($1, $2, $3)
    RETURNING *
), owner_member AS (
    INSERT INTO board_members (board_id, user_id, role)
    SELECT id, owner_id, 'owner' FROM new_board
    RETURNING board_id
)
SELECT new_board.* FROM new_board JOIN owner_member ON owner_member.board_id = new_board.id;

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
