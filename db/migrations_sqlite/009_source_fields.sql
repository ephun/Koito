-- +goose Up
CREATE TABLE source_field_catalog (
    source_id INTEGER NOT NULL REFERENCES source_accounts(id),
    path TEXT NOT NULL,
    kind TEXT NOT NULL,
    label TEXT NOT NULL,
    PRIMARY KEY(source_id,path,kind)
);
CREATE TABLE source_field_catalog_state (
    source_id INTEGER PRIMARY KEY REFERENCES source_accounts(id),
    record_count INTEGER NOT NULL
);

-- +goose Down
DROP TABLE source_field_catalog_state;
DROP TABLE source_field_catalog;
