-- +goose Up
CREATE TABLE listen_scope (
 track_id INTEGER NOT NULL,
 listened_at INTEGER NOT NULL,
 play_key TEXT NOT NULL,
 included INTEGER NOT NULL CHECK(included IN (0,1)),
 issues_json TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(issues_json)),
 PRIMARY KEY(track_id,listened_at,play_key),
 FOREIGN KEY(track_id,listened_at,play_key) REFERENCES listens(track_id,listened_at,play_key) ON DELETE CASCADE
);
DROP VIEW displayed_listens;
CREATE VIEW displayed_listens AS SELECT l.* FROM listens l
WHERE COALESCE(
 (SELECT s.included FROM listen_scope s WHERE s.track_id=l.track_id AND s.listened_at=l.listened_at AND s.play_key=l.play_key),
 (SELECT s.included FROM track_scope s WHERE s.track_id=l.track_id),1)=1;
-- +goose Down
DROP VIEW displayed_listens;
CREATE VIEW displayed_listens AS SELECT l.* FROM listens l
WHERE NOT EXISTS(SELECT 1 FROM track_scope s WHERE s.track_id=l.track_id AND s.included=0);
DROP TABLE listen_scope;
