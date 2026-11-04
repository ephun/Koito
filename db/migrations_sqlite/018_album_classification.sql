-- +goose Up
CREATE TABLE album_classification (
 album_id INTEGER PRIMARY KEY REFERENCES releases(id),
 various_artists INTEGER NOT NULL CHECK(various_artists IN (0,1))
);
CREATE TABLE album_classification_issues (
 album_id INTEGER PRIMARY KEY REFERENCES releases(id),
 issues_json TEXT NOT NULL CHECK(json_valid(issues_json))
);
-- +goose Down
DROP TABLE album_classification_issues;
DROP TABLE album_classification;
