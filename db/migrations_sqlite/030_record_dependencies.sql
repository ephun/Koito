-- +goose Up
ALTER TABLE source_personal_evidence_state ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
CREATE INDEX source_personal_evidence_record_music ON source_personal_evidence_music(record_id,music_key);
CREATE INDEX source_match_candidates_reverse ON source_match_candidates(candidate_id,record_id);
CREATE INDEX match_decisions_reverse ON match_decisions(right_record,left_record);
CREATE INDEX source_projection_play ON source_projection(play_key);
CREATE INDEX source_listens_play ON listens(play_key);
CREATE INDEX source_listens_identity ON listens(track_id,listened_at,user_id,play_key);
CREATE TABLE source_record_dependencies (
 record_id INTEGER PRIMARY KEY REFERENCES source_records(id),
 source_id INTEGER NOT NULL REFERENCES source_accounts(id),
 user_id INTEGER NOT NULL REFERENCES users(id),
 start_ms INTEGER,
 end_ms INTEGER,
 event_id TEXT NOT NULL,
 support_id TEXT NOT NULL,
 exact_key TEXT NOT NULL,
 eligible INTEGER NOT NULL CHECK(eligible IN (0,1))
);
CREATE INDEX source_record_dependencies_event ON source_record_dependencies(source_id,event_id) WHERE event_id<>'';
CREATE INDEX source_record_dependencies_support ON source_record_dependencies(source_id,support_id) WHERE support_id<>'';
CREATE INDEX source_record_dependencies_exact ON source_record_dependencies(exact_key) WHERE exact_key<>'';
CREATE TABLE source_record_music_keys (
 record_id INTEGER NOT NULL REFERENCES source_record_dependencies(record_id) ON DELETE CASCADE,
 music_key TEXT NOT NULL,
 PRIMARY KEY(music_key,record_id)
);
CREATE INDEX source_record_music_keys_record ON source_record_music_keys(record_id);
CREATE TABLE source_record_dependency_state (
 id INTEGER PRIMARY KEY CHECK(id=1),
 version_id INTEGER NOT NULL REFERENCES filter_versions(id),
 max_record_id INTEGER NOT NULL,
 max_source_id INTEGER NOT NULL,
 max_snapshot_id INTEGER NOT NULL,
 revision INTEGER NOT NULL
);
-- +goose Down
DROP TABLE source_record_dependency_state;
DROP TABLE source_record_music_keys;
DROP TABLE source_record_dependencies;
DROP INDEX match_decisions_reverse;
DROP INDEX source_projection_play;
DROP INDEX source_listens_play;
DROP INDEX source_listens_identity;
DROP INDEX source_match_candidates_reverse;
DROP INDEX source_personal_evidence_record_music;
ALTER TABLE source_personal_evidence_state DROP COLUMN revision;
