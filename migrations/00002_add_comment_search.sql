-- +goose Up
CREATE TEXT SEARCH CONFIGURATION public.comment_search (COPY = pg_catalog.russian);
-- ASCII words use English stemming; other word mappings retain Russian stemming.
ALTER TEXT SEARCH CONFIGURATION public.comment_search
    ALTER MAPPING FOR asciiword, asciihword, hword_asciipart
    WITH pg_catalog.english_stem;

ALTER TABLE comments ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('public.comment_search'::regconfig, body)) STORED;
CREATE INDEX comments_search_vector_idx ON comments USING gin (search_vector);

-- +goose Down
DROP INDEX comments_search_vector_idx;
ALTER TABLE comments DROP COLUMN search_vector;
DROP TEXT SEARCH CONFIGURATION public.comment_search;
