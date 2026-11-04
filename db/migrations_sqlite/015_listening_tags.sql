-- +goose Up
CREATE TABLE source_play_tags (
 play_key TEXT PRIMARY KEY,
 tags_json TEXT NOT NULL CHECK(json_valid(tags_json))
);
-- +goose Down
DROP TABLE source_play_tags;
