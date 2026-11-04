-- +goose Up
CREATE TABLE album_groups (
    id INTEGER PRIMARY KEY,
    group_key TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL,
    canonical_release_id INTEGER REFERENCES releases(id)
);
CREATE TABLE album_group_members (
    group_id INTEGER NOT NULL REFERENCES album_groups(id),
    release_id INTEGER NOT NULL REFERENCES releases(id),
    PRIMARY KEY(group_id,release_id)
);
CREATE INDEX album_group_members_release ON album_group_members(release_id);
CREATE TABLE native_release_groups (
    release_id INTEGER PRIMARY KEY REFERENCES releases(id),
    group_id INTEGER NOT NULL REFERENCES album_groups(id)
);
-- Zero explicitly keeps a source contribution outside a native release group.
CREATE TABLE source_play_groupings (play_key TEXT PRIMARY KEY, group_id INTEGER NOT NULL);
CREATE TABLE source_aggregate_groupings (aggregate_key TEXT PRIMARY KEY, group_id INTEGER NOT NULL);
CREATE VIEW album_report_events AS
SELECT l.*,t.release_id,COALESCE(s.group_id,n.group_id,0) AS album_group_id
FROM listens l JOIN tracks t ON t.id=l.track_id
LEFT JOIN source_play_groupings s ON s.play_key=l.play_key
LEFT JOIN native_release_groups n ON n.release_id=t.release_id;
CREATE VIEW album_report_counts AS
SELECT track_id,user_id,listened_at,release_id,album_group_id,1 AS amount FROM album_report_events
UNION ALL
SELECT a.track_id,a.user_id,NULL,t.release_id,COALESCE(s.group_id,n.group_id,0),a.undated_count
FROM source_aggregates a JOIN tracks t ON t.id=a.track_id
LEFT JOIN source_aggregate_groupings s ON s.aggregate_key=a.aggregate_key
LEFT JOIN native_release_groups n ON n.release_id=t.release_id
WHERE a.undated_count>0;

-- +goose Down
DROP VIEW album_report_counts;
DROP VIEW album_report_events;
DROP TABLE source_aggregate_groupings;
DROP TABLE source_play_groupings;
DROP TABLE native_release_groups;
DROP TABLE album_group_members;
DROP TABLE album_groups;
