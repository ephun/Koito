-- +goose Up
CREATE TABLE source_history_variants (
    base_track_id INTEGER NOT NULL REFERENCES tracks(id),
    comparison_key TEXT NOT NULL,
    track_id INTEGER NOT NULL REFERENCES tracks(id),
    PRIMARY KEY(base_track_id,comparison_key)
);
CREATE INDEX source_history_variants_track ON source_history_variants(track_id);
CREATE TABLE track_history_overrides (
    track_id INTEGER PRIMARY KEY REFERENCES tracks(id),
    comparison_key TEXT NOT NULL
);

-- +goose Down
DROP TABLE track_history_overrides;
DROP TABLE source_history_variants;
