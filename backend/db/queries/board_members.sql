-- name: AddBoardMember :exec
INSERT INTO board_members (board_id, user_id, role)
VALUES ($1, $2, $3);

-- name: RemoveBoardMember :exec
DELETE FROM board_members WHERE board_id = $1 AND user_id = $2;

-- name: GetBoardMember :one
SELECT * FROM board_members WHERE board_id = $1 AND user_id = $2;

-- name: ListBoardMembers :many
SELECT * FROM board_members WHERE board_id = $1
ORDER BY created_at ASC;
