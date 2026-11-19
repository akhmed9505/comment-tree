-- +goose Up
CREATE TABLE comments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    parent_id bigint,
    author text NOT NULL,
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT comments_id_positive CHECK (id > 0),
    -- Parents precede replies, so decreasing parent IDs make cycles impossible.
    CONSTRAINT comments_parent_precedes_child CHECK (parent_id < id),
    CONSTRAINT comments_parent_id_fkey FOREIGN KEY (parent_id)
        REFERENCES comments (id) ON DELETE CASCADE,
    -- The whitespace set matches Go's strings.TrimSpace; btrim defaults to ASCII space.
    CONSTRAINT comments_author_valid CHECK (
        char_length(author) BETWEEN 1 AND 80 AND
        btrim(author, U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') <> ''
    ),
    CONSTRAINT comments_body_valid CHECK (
        char_length(body) BETWEEN 1 AND 10000 AND
        btrim(body, U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000') <> ''
    )
);

CREATE INDEX comments_roots_created_at_id_idx ON comments (created_at, id)
    WHERE parent_id IS NULL;
CREATE INDEX comments_parent_created_at_id_idx ON comments (parent_id, created_at, id)
    WHERE parent_id IS NOT NULL;

-- +goose Down
DROP TABLE comments;
