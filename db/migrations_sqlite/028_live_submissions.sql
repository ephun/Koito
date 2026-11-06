-- +goose Up
CREATE TABLE live_submissions (
    id TEXT PRIMARY KEY,
    source_id INTEGER NOT NULL REFERENCES source_accounts(id),
    file_hash TEXT NOT NULL REFERENCES source_files(hash),
    listen_type TEXT NOT NULL,
    received_at TEXT NOT NULL,
    request_key TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'stored' CHECK(status IN ('stored','projected')),
    version_id INTEGER REFERENCES filter_versions(id)
);
CREATE INDEX live_submissions_pending ON live_submissions(status,received_at);
CREATE TABLE live_submission_records (
    submission_id TEXT NOT NULL REFERENCES live_submissions(id),
    position INTEGER NOT NULL,
    record_id INTEGER NOT NULL REFERENCES source_records(id),
    PRIMARY KEY(submission_id,position)
);
CREATE INDEX live_submission_records_record ON live_submission_records(record_id);

-- +goose Down
DROP TABLE live_submission_records;
DROP TABLE live_submissions;
