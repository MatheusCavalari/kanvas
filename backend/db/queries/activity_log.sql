-- name: InsertActivityEntry :exec
INSERT INTO activity_log (id, board_id, actor_id, action, entity_type, entity_id, snapshot_before, snapshot_after)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListActivityByBoard :many
SELECT * FROM activity_log
WHERE board_id = $1 AND created_at < $2
ORDER BY created_at DESC
LIMIT $3;
