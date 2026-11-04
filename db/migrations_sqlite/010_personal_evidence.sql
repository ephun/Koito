-- +goose Up
CREATE TABLE source_personal_evidence (
    play_key TEXT NOT NULL,
    record_id INTEGER NOT NULL REFERENCES source_records(id),
    details_json TEXT NOT NULL CHECK(json_valid(details_json)),
    PRIMARY KEY(play_key,record_id)
);
CREATE TABLE source_personal_evidence_music (
    music_key TEXT NOT NULL,
    play_key TEXT NOT NULL,
    record_id INTEGER NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY(music_key,play_key,record_id),
    FOREIGN KEY(play_key,record_id) REFERENCES source_personal_evidence(play_key,record_id) ON DELETE CASCADE
);
CREATE INDEX source_personal_evidence_music_order ON source_personal_evidence_music(music_key,position);
CREATE INDEX source_personal_evidence_music_record ON source_personal_evidence_music(play_key,record_id);
CREATE TABLE source_personal_evidence_state (
    id INTEGER PRIMARY KEY CHECK(id=1),
    version_id INTEGER NOT NULL REFERENCES filter_versions(id)
);

-- +goose Down
DROP TABLE source_personal_evidence_state;
DROP TABLE source_personal_evidence_music;
DROP TABLE source_personal_evidence;
