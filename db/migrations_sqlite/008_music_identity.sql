-- +goose Up
-- Existing tracks remain album appearances. Independent histories collect
-- those appearances without redirecting, deleting, or combining listens.
CREATE TABLE music_tracks (
    id INTEGER PRIMARY KEY,
    identity_key TEXT NOT NULL UNIQUE
);
CREATE TABLE track_histories (
    track_id INTEGER PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
    music_track_id INTEGER NOT NULL REFERENCES music_tracks(id)
);
CREATE INDEX track_histories_music ON track_histories(music_track_id,track_id);

-- A recording identity and a name-based listening history are distinct.
-- Unidentified appearances keep their own recording identity.
CREATE TABLE recordings (
    recording_key TEXT PRIMARY KEY,
    musicbrainz_id TEXT,
    spotify_uri TEXT
);
CREATE INDEX recordings_mbid ON recordings(musicbrainz_id);
CREATE INDEX recordings_spotify ON recordings(spotify_uri);
CREATE TABLE track_recordings (
    track_id INTEGER PRIMARY KEY REFERENCES tracks(id) ON DELETE CASCADE,
    recording_key TEXT NOT NULL REFERENCES recordings(recording_key)
);
CREATE INDEX track_recordings_recording ON track_recordings(recording_key,track_id);
INSERT INTO recordings(recording_key,musicbrainz_id)
SELECT CASE WHEN musicbrainz_id IS NOT NULL THEN 'mbid:'||musicbrainz_id ELSE 'appearance:'||id END,musicbrainz_id
FROM tracks;
INSERT INTO track_recordings(track_id,recording_key)
SELECT id,CASE WHEN musicbrainz_id IS NOT NULL THEN 'mbid:'||musicbrainz_id ELSE 'appearance:'||id END FROM tracks;

CREATE VIEW track_history_members AS
SELECT h.track_id,h.music_track_id,
    (SELECT MIN(m.track_id) FROM track_histories m WHERE m.music_track_id=h.music_track_id) AS canonical_track_id
FROM track_histories h
UNION ALL
SELECT t.id,NULL,t.id FROM tracks t WHERE NOT EXISTS(SELECT 1 FROM track_histories h WHERE h.track_id=t.id);

-- +goose Down
-- +goose StatementBegin
CREATE TEMP TABLE koito_music_rollback_guard (id INTEGER);
CREATE TEMP TRIGGER koito_music_requires_backup BEFORE INSERT ON koito_music_rollback_guard BEGIN
    SELECT RAISE(ABORT,'Restore a pre-migration backup to remove music identities.');
END;
INSERT INTO koito_music_rollback_guard VALUES(1);
-- +goose StatementEnd
