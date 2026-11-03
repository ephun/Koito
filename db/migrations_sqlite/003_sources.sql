-- +goose Up
CREATE TABLE source_accounts (
    id INTEGER PRIMARY KEY,
    service TEXT NOT NULL,
    account TEXT NOT NULL,
    user_id INTEGER NOT NULL REFERENCES users(id),
    UNIQUE(service, account)
);
CREATE TABLE source_files (
    hash TEXT PRIMARY KEY CHECK(length(hash)=64),
    size INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE TABLE source_members (
    parent_hash TEXT NOT NULL REFERENCES source_files(hash),
    position INTEGER NOT NULL,
    name TEXT NOT NULL,
    file_hash TEXT NOT NULL REFERENCES source_files(hash),
    PRIMARY KEY(parent_hash, position)
);
CREATE TABLE import_receipts (
    id INTEGER PRIMARY KEY,
    source_id INTEGER NOT NULL REFERENCES source_accounts(id),
    file_hash TEXT NOT NULL REFERENCES source_files(hash),
    filename TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('processing','stored','failed')),
    error TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
CREATE TABLE source_extractions (
    source_id INTEGER NOT NULL REFERENCES source_accounts(id),
    file_hash TEXT NOT NULL REFERENCES source_files(hash),
    parser TEXT NOT NULL,
    record_count INTEGER NOT NULL,
    PRIMARY KEY(source_id, file_hash, parser)
);
CREATE TABLE source_records (
    id INTEGER PRIMARY KEY,
    source_id INTEGER NOT NULL REFERENCES source_accounts(id),
    file_hash TEXT NOT NULL REFERENCES source_files(hash),
    ordinal INTEGER NOT NULL,
    kind TEXT NOT NULL,
    payload TEXT NOT NULL,
    parser TEXT NOT NULL,
    UNIQUE(source_id,file_hash,parser,ordinal)
);
CREATE INDEX source_records_source ON source_records(source_id,id);
CREATE TABLE source_interpretations (
    record_id INTEGER NOT NULL REFERENCES source_records(id),
    policy TEXT NOT NULL,
    play_key TEXT NOT NULL,
    reason TEXT NOT NULL,
    PRIMARY KEY(record_id,policy)
);
CREATE TABLE source_projection (
    track_id INTEGER NOT NULL,
    listened_at INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    policy TEXT NOT NULL,
    owned INTEGER NOT NULL,
    PRIMARY KEY(track_id,listened_at)
);
CREATE TABLE source_plays (
    policy TEXT NOT NULL,
    play_key TEXT NOT NULL,
    source_id INTEGER NOT NULL REFERENCES source_accounts(id),
    representative_record INTEGER NOT NULL REFERENCES source_records(id),
    track_id INTEGER,
    listened_at INTEGER NOT NULL,
    disposition TEXT NOT NULL,
    PRIMARY KEY(policy,play_key)
);
CREATE TABLE source_projection_runs (
    id INTEGER PRIMARY KEY,
    policy TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    records INTEGER NOT NULL,
    plays INTEGER NOT NULL,
    collisions INTEGER NOT NULL
);
-- Raw evidence is append-only. Interpretations and projections are rebuildable.
-- +goose StatementBegin
CREATE TRIGGER source_records_no_update BEFORE UPDATE ON source_records BEGIN
    SELECT RAISE(ABORT,'source records are immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER source_records_no_delete BEFORE DELETE ON source_records BEGIN
    SELECT RAISE(ABORT,'source records are immutable');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER source_records_no_delete;
DROP TRIGGER source_records_no_update;
DROP TABLE source_projection_runs;
DROP TABLE source_plays;
DROP TABLE source_projection;
DROP TABLE source_interpretations;
DROP TABLE source_records;
DROP TABLE source_extractions;
DROP TABLE import_receipts;
DROP TABLE source_members;
DROP TABLE source_files;
DROP TABLE source_accounts;
