-- name: AttachLabel :exec
INSERT INTO card_labels (card_id, label_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: DetachLabel :exec
DELETE FROM card_labels WHERE card_id = $1 AND label_id = $2;

-- name: ListLabelsByCard :many
SELECT l.* FROM labels l
JOIN card_labels cl ON cl.label_id = l.id
WHERE cl.card_id = $1
ORDER BY l.name ASC;
