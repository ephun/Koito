-- +goose Up
CREATE TABLE listens_with_keys (
    track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    listened_at INTEGER NOT NULL,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client TEXT NOT NULL DEFAULT '',
    play_key TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(track_id,listened_at,play_key)
);
INSERT INTO listens_with_keys(track_id,listened_at,user_id,client)
SELECT track_id,listened_at,user_id,client FROM listens;
DROP TABLE listens;
ALTER TABLE listens_with_keys RENAME TO listens;
CREATE INDEX idx_listens_listened_at ON listens(listened_at);
CREATE INDEX idx_listens_track_id ON listens(track_id);
CREATE INDEX idx_listens_track_id_listened_at ON listens(track_id,listened_at);
CREATE INDEX idx_listens_user_id ON listens(user_id);
CREATE TABLE source_projection_with_keys (
    track_id INTEGER NOT NULL,
    listened_at INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    policy TEXT NOT NULL,
    owned INTEGER NOT NULL,
    play_key TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(track_id,listened_at,play_key)
);
INSERT INTO source_projection_with_keys(track_id,listened_at,user_id,policy,owned)
SELECT track_id,listened_at,user_id,policy,owned FROM source_projection;
DROP TABLE source_projection;
ALTER TABLE source_projection_with_keys RENAME TO source_projection;
ALTER TABLE source_plays ADD COLUMN version_id INTEGER REFERENCES filter_versions(id);
CREATE INDEX source_plays_representative ON source_plays(policy,representative_record);
CREATE INDEX source_interpretations_play ON source_interpretations(policy,play_key);
CREATE TABLE source_match_candidates (
    record_id INTEGER NOT NULL REFERENCES source_records(id),
    candidate_id INTEGER NOT NULL REFERENCES source_records(id),
    PRIMARY KEY(record_id,candidate_id)
);

-- +goose Down
-- Collapsing distinct plays would lose information. Restore a pre-migration
-- backup instead of automatically undoing this migration.
-- +goose StatementBegin
CREATE TEMP TABLE koito_rollback_guard (id INTEGER);
CREATE TEMP TRIGGER koito_rollback_requires_backup BEFORE INSERT ON koito_rollback_guard BEGIN
    SELECT RAISE(ABORT,'Restore a pre-migration backup to remove play identity.');
END;
INSERT INTO koito_rollback_guard VALUES(1);
-- +goose StatementEnd
