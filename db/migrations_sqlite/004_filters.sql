-- +goose Up
CREATE TABLE filter_versions (
    id INTEGER PRIMARY KEY,
    parent_id INTEGER REFERENCES filter_versions(id),
    config_json TEXT NOT NULL,
    label TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
INSERT INTO filter_versions(id,config_json,label,created_at) VALUES(1,
'{"rules":[{"id":"music","type":"matching","subtype":"music","enabled":true,"scope":{},"settings":{"case_sensitive":true,"match_missing_album":true}},{"id":"plays","type":"matching","subtype":"play","enabled":true,"scope":{},"settings":{"tolerance_seconds":0,"spotify_time":"estimated_start","preferred_source":"spotify"}},{"id":"wall-time","type":"inference","subtype":"time","enabled":true,"scope":{"service":"lastfm"},"settings":{"timezone":""}}]}',
'Initial rules',CAST(strftime('%s','now') AS INTEGER));
CREATE TABLE filter_state (
    id INTEGER PRIMARY KEY CHECK(id=1),
    version_id INTEGER NOT NULL REFERENCES filter_versions(id)
);
INSERT INTO filter_state VALUES(1,1);
CREATE TABLE match_decisions (
    id INTEGER PRIMARY KEY,
    left_record INTEGER NOT NULL REFERENCES source_records(id),
    right_record INTEGER NOT NULL REFERENCES source_records(id),
    action TEXT NOT NULL CHECK(action IN ('match','split','clear')),
    created_at INTEGER NOT NULL,
    CHECK(left_record<right_record)
);
CREATE INDEX match_decisions_pair ON match_decisions(left_record,right_record,id);
-- Saved rule versions and manual decisions are append-only. Undo adds a version.
-- +goose StatementBegin
CREATE TRIGGER filter_versions_no_update BEFORE UPDATE ON filter_versions BEGIN
 SELECT RAISE(ABORT,'saved rule versions are immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER filter_versions_no_delete BEFORE DELETE ON filter_versions BEGIN
 SELECT RAISE(ABORT,'saved rule versions are immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER match_decisions_no_update BEFORE UPDATE ON match_decisions BEGIN
 SELECT RAISE(ABORT,'manual decisions are immutable');
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER match_decisions_no_delete BEFORE DELETE ON match_decisions BEGIN
 SELECT RAISE(ABORT,'manual decisions are immutable');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER match_decisions_no_delete;
DROP TRIGGER match_decisions_no_update;
DROP TRIGGER filter_versions_no_delete;
DROP TRIGGER filter_versions_no_update;
DROP TABLE match_decisions;
DROP TABLE filter_state;
DROP TABLE filter_versions;
