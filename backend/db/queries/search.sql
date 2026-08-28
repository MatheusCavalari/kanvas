-- name: SearchCards :many
SELECT c.id, c.title, c.column_id, ts_rank(c.search_vector, plainto_tsquery('english', $1)) AS rank
FROM cards c
JOIN columns col ON col.id = c.column_id
WHERE col.board_id = $2 AND c.search_vector @@ plainto_tsquery('english', $1)
ORDER BY rank DESC
LIMIT 20;

-- name: SearchComments :many
SELECT cm.id, cm.card_id,
    ts_headline('english', cm.body, plainto_tsquery('english', $1), 'MaxWords=30,MinWords=15') AS body_excerpt,
    ts_rank(cm.search_vector, plainto_tsquery('english', $1)) AS rank
FROM comments cm
JOIN cards c ON c.id = cm.card_id
JOIN columns col ON col.id = c.column_id
WHERE col.board_id = $2 AND cm.search_vector @@ plainto_tsquery('english', $1)
ORDER BY rank DESC
LIMIT 20;
