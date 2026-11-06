-- +goose Up
CREATE TABLE source_play_context (
    play_key TEXT PRIMARY KEY REFERENCES source_play_details(play_key) ON DELETE CASCADE,
    context_json TEXT NOT NULL CHECK(json_valid(context_json))
);

-- +goose Down
DROP TABLE source_play_context;
