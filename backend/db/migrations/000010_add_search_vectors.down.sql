DROP INDEX IF EXISTS idx_comments_search;
ALTER TABLE comments DROP COLUMN IF EXISTS search_vector;
DROP INDEX IF EXISTS idx_cards_search;
ALTER TABLE cards DROP COLUMN IF EXISTS search_vector;
