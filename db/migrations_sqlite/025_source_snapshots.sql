-- +goose Up
CREATE TABLE source_snapshots (
 id INTEGER PRIMARY KEY,
 source_id INTEGER NOT NULL REFERENCES source_accounts(id),
 file_hash TEXT NOT NULL REFERENCES source_files(hash),
 parser TEXT NOT NULL,
 set_key TEXT NOT NULL,
 group_key TEXT NOT NULL DEFAULT '',
 kind TEXT NOT NULL,
 name TEXT NOT NULL,
 metadata TEXT NOT NULL,
 UNIQUE(source_id,file_hash,parser,set_key)
);
CREATE INDEX source_snapshots_current ON source_snapshots(source_id,set_key,id);
CREATE INDEX source_snapshots_group ON source_snapshots(source_id,group_key,id);
CREATE TABLE source_snapshot_items (
 snapshot_id INTEGER NOT NULL REFERENCES source_snapshots(id),
 position INTEGER NOT NULL,
 record_id INTEGER NOT NULL REFERENCES source_records(id),
 PRIMARY KEY(snapshot_id,position)
);
CREATE INDEX source_snapshot_items_record ON source_snapshot_items(record_id);
CREATE TABLE source_music_states (
 record_id INTEGER PRIMARY KEY REFERENCES source_records(id),
 snapshot_id INTEGER NOT NULL REFERENCES source_snapshots(id),
 track_id INTEGER REFERENCES tracks(id),
 release_id INTEGER REFERENCES releases(id),
 artist_id INTEGER REFERENCES artists(id),
 external_id TEXT NOT NULL DEFAULT '',
 kind TEXT NOT NULL,
 name TEXT NOT NULL,
 position INTEGER NOT NULL
);
CREATE INDEX source_music_states_track ON source_music_states(track_id);
CREATE INDEX source_music_states_release ON source_music_states(release_id);
CREATE TABLE source_release_ids (
 service TEXT NOT NULL,
 external_id TEXT NOT NULL,
 release_id INTEGER NOT NULL REFERENCES releases(id),
 PRIMARY KEY(service,external_id)
);
-- +goose StatementBegin
CREATE TRIGGER source_snapshots_no_update BEFORE UPDATE ON source_snapshots BEGIN SELECT RAISE(ABORT,'source snapshots are immutable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER source_snapshots_no_delete BEFORE DELETE ON source_snapshots BEGIN SELECT RAISE(ABORT,'source snapshots are immutable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER source_snapshot_items_no_update BEFORE UPDATE ON source_snapshot_items BEGIN SELECT RAISE(ABORT,'snapshot items are immutable'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER source_snapshot_items_no_delete BEFORE DELETE ON source_snapshot_items BEGIN SELECT RAISE(ABORT,'snapshot items are immutable'); END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER source_snapshot_items_no_delete;
DROP TRIGGER source_snapshot_items_no_update;
DROP TRIGGER source_snapshots_no_delete;
DROP TRIGGER source_snapshots_no_update;
DROP TABLE source_release_ids;
DROP TABLE source_music_states;
DROP TABLE source_snapshot_items;
DROP TABLE source_snapshots;
