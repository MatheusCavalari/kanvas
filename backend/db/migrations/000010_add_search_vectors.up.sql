ALTER TABLE cards ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', title || ' ' || coalesce(description, ''))) STORED;
CREATE INDEX idx_cards_search ON cards USING GIN(search_vector);

ALTER TABLE comments ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', body)) STORED;
CREATE INDEX idx_comments_search ON comments USING GIN(search_vector);
