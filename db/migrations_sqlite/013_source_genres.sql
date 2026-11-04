-- +goose Up
CREATE TABLE source_genres (
    record_id INTEGER NOT NULL REFERENCES source_records(id),
    position INTEGER NOT NULL,
    field TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY(record_id,position)
);
CREATE INDEX source_genres_value ON source_genres(value);

-- +goose Down
DROP TABLE source_genres;
