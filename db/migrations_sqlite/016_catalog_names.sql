-- +goose Up
CREATE TABLE catalog_names (
 kind TEXT NOT NULL CHECK(kind IN ('artist','album','track')),
 item_id INTEGER NOT NULL,
 display_name TEXT NOT NULL,
 PRIMARY KEY(kind,item_id)
);
CREATE TABLE catalog_name_issues (
 kind TEXT NOT NULL,
 item_id INTEGER NOT NULL,
 issues_json TEXT NOT NULL CHECK(json_valid(issues_json)),
 PRIMARY KEY(kind,item_id)
);
-- +goose Down
DROP TABLE catalog_name_issues;
DROP TABLE catalog_names;
