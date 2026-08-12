-- name: AddBoardMember :exec
INSERT INTO board_members (board_id, user_id, role)
VALUES ($1, $2, $3);

-- name: RemoveBoardMember :exec
DELETE FROM board_members WHERE board_id = $1 AND user_id = $2;

-- name: GetBoardMember :one
SELECT * FROM board_members WHERE board_id = $1 AND user_id = $2;

-- name: ListBoardMembers :many
SELECT bm.board_id, bm.user_id, bm.role, bm.created_at, u.name, u.email
FROM board_members bm
JOIN users u ON u.id = bm.user_id
WHERE bm.board_id = $1
ORDER BY bm.created_at ASC;
